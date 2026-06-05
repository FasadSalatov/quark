// Package quark — Go reference implementation of the Quark Protocol v0.2.
//
// Spec: https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
//
// v0.2 features:
//   - Cryptographically signed capability tokens (QCT, HMAC-SHA256)
//   - Bearer auth in handshake
//   - Session resume after disconnect (RSM)
//   - Heartbeat (HBT/HBA)
//   - Tool input validation (JSON Schema)
//   - Cost tracking in responses
//   - Distributed tracing (W3C-style trace/span IDs)
//   - Tool versioning (name@version syntax)
//   - Backwards-compatible adapter for v0.1 clients
package quark

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultSessionTTL       = time.Hour
	DefaultHeartbeatTimeout = 90 * time.Second
	ReplayBufferSize        = 64
)

// ───────────────────────────────────────────────────────────────
// Public types
// ───────────────────────────────────────────────────────────────

// ServerOptions configures the Quark server.
type ServerOptions struct {
	// Secret is the HMAC key used to verify QCT tokens. Required for auth.
	Secret []byte
	// SessionTTL determines how long sessions are kept after disconnect.
	SessionTTL time.Duration
	// AllowAnonymous controls whether clients without auth can connect.
	// If true, anonymous channels are limited (no subscriptions, no resume,
	// only effects: pure|read).
	AllowAnonymous bool
}

// Tool describes a function exposed via Quark.
type Tool struct {
	Name        string
	Version     string // optional; "v2" suffix in name takes precedence
	Description string
	Input       map[string]any // JSON Schema
	Output      map[string]any
	Effects     []string
	Cost        *CostEstimate
	Streaming   bool
	Capability  string

	// One-shot handler: returns output, optional cost, error.
	Handler func(ctx context.Context, input map[string]any) (any, *Cost, error)

	// Streaming handler: send chunks via channel; return final cost on END.
	StreamHandler func(ctx context.Context, input map[string]any, chunks chan<- any) (*Cost, error)
}

// CostEstimate is the advertised per-call cost.
type CostEstimate struct {
	Estimate float64 `json:"estimate"`
	Currency string  `json:"currency"`
}

// Cost is reported in RES/END to tell the agent how much a call really cost.
type Cost struct {
	ComputeMs int     `json:"compute_ms,omitempty"`
	APICalls  int     `json:"api_calls,omitempty"`
	USD       float64 `json:"usd,omitempty"`
	Tokens    int     `json:"tokens,omitempty"`
}

// TopicHandler implements subscriptions.
type TopicHandler func(ctx context.Context, filter map[string]any, events chan<- any) error

// QCT (Quark Capability Token) payload.
type QCTPayload struct {
	Issuer      string   `json:"iss"`
	Subject     string   `json:"sub"`
	IssuedAt    int64    `json:"iat"`
	NotBefore   int64    `json:"nbf,omitempty"`
	Expiry      int64    `json:"exp"`
	Scope       []string `json:"scope"`
	ClientID    string   `json:"client_id,omitempty"`
	SessionID   string   `json:"session_id,omitempty"`
	MaxCostUSD  float64  `json:"max_cost_usd,omitempty"`
}

// ───────────────────────────────────────────────────────────────
// Server
// ───────────────────────────────────────────────────────────────

type Server struct {
	opts     *ServerOptions
	tools    map[string]Tool
	topics   map[string]TopicHandler
	upgrader websocket.Upgrader
	sessions sync.Map // session_id -> *session
	mu       sync.RWMutex
}

func NewServer(opts *ServerOptions) *Server {
	if opts == nil {
		opts = &ServerOptions{}
	}
	if opts.SessionTTL == 0 {
		opts.SessionTTL = DefaultSessionTTL
	}
	return &Server{
		opts:   opts,
		tools:  map[string]Tool{},
		topics: map[string]TopicHandler{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) RegisterTool(t Tool) {
	if t.Name == "" || (t.Handler == nil && t.StreamHandler == nil) {
		return
	}
	key := t.Name
	if t.Version != "" && !strings.Contains(t.Name, "@") {
		key = t.Name + "@" + t.Version
	}
	s.mu.Lock()
	s.tools[key] = t
	s.mu.Unlock()
}

func (s *Server) RegisterTopic(name string, handler TopicHandler) {
	if name == "" || handler == nil {
		return
	}
	s.mu.Lock()
	s.topics[name] = handler
	s.mu.Unlock()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("quark: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ch := &channel{
		server:        s,
		conn:          conn,
		writes:        make(chan map[string]any, 64),
		active:        map[int]context.CancelFunc{},
		grants:        map[string]bool{},
		replayBuf:     newReplayBuffer(ReplayBufferSize),
		lastHeartbeat: time.Now(),
	}
	go ch.writer()
	go ch.heartbeatMonitor()
	ch.reader()

	// On disconnect, parking session for TTL.
	if ch.sessionID != "" {
		ch.parkSession()
	}
}

// ───────────────────────────────────────────────────────────────
// Session (parked state across disconnects)
// ───────────────────────────────────────────────────────────────

type session struct {
	id            string
	agentID       string
	grants        map[string]bool
	replayBuf     *replayBuffer
	subscriptions map[int]subscription
	expireAt      time.Time
	costAccum     Cost
	maxCostUSD    float64
	mu            sync.Mutex
}

type subscription struct {
	topic  string
	filter map[string]any
	cancel context.CancelFunc
}

func (s *Server) parkSession(sess *session) {
	sess.expireAt = time.Now().Add(s.opts.SessionTTL)
	s.sessions.Store(sess.id, sess)
}

func (s *Server) lookupSession(id string) *session {
	v, ok := s.sessions.Load(id)
	if !ok {
		return nil
	}
	sess := v.(*session)
	if time.Now().After(sess.expireAt) {
		s.sessions.Delete(id)
		return nil
	}
	return sess
}

// ───────────────────────────────────────────────────────────────
// Channel (per-connection)
// ───────────────────────────────────────────────────────────────

type channel struct {
	server        *Server
	conn          *websocket.Conn
	writes        chan map[string]any
	mu            sync.Mutex
	active        map[int]context.CancelFunc
	grants        map[string]bool
	agentID       string
	sessionID     string
	authenticated bool
	maxCostUSD    float64
	costAccum     Cost
	replayBuf     *replayBuffer
	lastSeq       int
	lastHeartbeat time.Time
	subscriptions map[int]subscription
}

func (ch *channel) send(payload map[string]any) {
	if ch.replayBuf != nil {
		ch.replayBuf.add(payload)
	}
	select {
	case ch.writes <- payload:
	default:
	}
}

func (ch *channel) sendErr(seq int, code, msg string) {
	ch.send(map[string]any{
		"v":       ProtocolVersion,
		"kind":    "ERR",
		"seq":     seq,
		"code":    code,
		"message": msg,
	})
}

func (ch *channel) writer() {
	for f := range ch.writes {
		b, _ := json.Marshal(f)
		if err := ch.conn.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}

func (ch *channel) heartbeatMonitor() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		ch.mu.Lock()
		last := ch.lastHeartbeat
		ch.mu.Unlock()
		if time.Since(last) > DefaultHeartbeatTimeout {
			ch.conn.Close()
			return
		}
	}
}

func (ch *channel) reader() {
	defer close(ch.writes)
	for {
		_, raw, err := ch.conn.ReadMessage()
		if err != nil {
			return
		}
		var f map[string]any
		if err := json.Unmarshal(raw, &f); err != nil {
			ch.sendErr(0, "INVALID_FRAME", err.Error())
			continue
		}
		kind, _ := f["kind"].(string)
		seq := intVal(f["seq"])

		ch.mu.Lock()
		ch.lastHeartbeat = time.Now()
		if seq > ch.lastSeq {
			ch.lastSeq = seq
		}
		ch.mu.Unlock()

		switch kind {
		case "HEY":
			ch.handleHey(f)
		case "RSM":
			ch.handleResume(f)
		case "LST":
			ch.handleList(seq)
		case "INV":
			go ch.handleInvoke(seq, f)
		case "SUB":
			go ch.handleSubscribe(seq, f)
		case "UNS", "CAN":
			ch.handleCancel(seq)
		case "HBT":
			ch.send(map[string]any{
				"v":    ProtocolVersion,
				"kind": "HBA",
				"ts":   f["ts"],
			})
		case "BYE":
			return
		default:
			ch.sendErr(seq, "UNKNOWN_KIND", "kind: "+kind)
		}
	}
}

// ───────────────────────────────────────────────────────────────
// Handlers
// ───────────────────────────────────────────────────────────────

func (ch *channel) handleHey(f map[string]any) {
	if agent, ok := f["agent"].(map[string]any); ok {
		if id, ok := agent["id"].(string); ok {
			ch.agentID = id
		}
	}

	// Auth
	if auth, ok := f["auth"].(map[string]any); ok {
		if authType, _ := auth["type"].(string); authType == "bearer" {
			token, _ := auth["token"].(string)
			payload, err := VerifyQCT(token, ch.server.opts.Secret)
			if err != nil {
				ch.sendErr(0, "AUTH_INVALID", err.Error())
				ch.conn.Close()
				return
			}
			// Client ID pinning
			if payload.ClientID != "" && payload.ClientID != ch.agentID {
				ch.sendErr(0, "AUTH_INVALID", "client_id mismatch")
				ch.conn.Close()
				return
			}
			ch.mu.Lock()
			for _, scope := range payload.Scope {
				ch.grants[scope] = true
			}
			ch.maxCostUSD = payload.MaxCostUSD
			ch.authenticated = true
			ch.mu.Unlock()
		}
	} else if !ch.server.opts.AllowAnonymous {
		ch.sendErr(0, "AUTH_INVALID", "anonymous not allowed")
		ch.conn.Close()
		return
	}

	// Issue session ID
	ch.sessionID = newSessionID()

	ch.server.mu.RLock()
	toolCount := len(ch.server.tools)
	topicCount := len(ch.server.topics)
	ch.server.mu.RUnlock()

	ch.mu.Lock()
	granted := make([]string, 0, len(ch.grants))
	for c := range ch.grants {
		granted = append(granted, c)
	}
	ch.mu.Unlock()

	ch.send(map[string]any{
		"v":    ProtocolVersion,
		"kind": "HEY",
		"server": map[string]any{
			"id":      "unyly-quark-ref",
			"name":    "Unyly Quark Reference Server",
			"version": "1.0.0",
		},
		"supports":             []string{"streaming", "subscribe", "compose", "capabilities", "resume", "tracing", "heartbeat", "validation"},
		"session_id":           ch.sessionID,
		"session_ttl":          int(ch.server.opts.SessionTTL.Seconds()),
		"tools":                toolCount,
		"topics":               topicCount,
		"granted_capabilities": granted,
	})
}

func (ch *channel) handleResume(f map[string]any) {
	sessID, _ := f["session_id"].(string)
	lastSeq := intVal(f["last_seq_received"])

	sess := ch.server.lookupSession(sessID)
	if sess == nil {
		ch.sendErr(0, "SESSION_EXPIRED", "session not found")
		ch.conn.Close()
		return
	}

	// Restore state
	ch.mu.Lock()
	ch.sessionID = sess.id
	ch.agentID = sess.agentID
	ch.grants = sess.grants
	ch.maxCostUSD = sess.maxCostUSD
	ch.costAccum = sess.costAccum
	ch.replayBuf = sess.replayBuf
	ch.authenticated = true
	ch.mu.Unlock()

	// Replay missed frames
	missed := ch.replayBuf.since(lastSeq)
	for _, f := range missed {
		ch.send(f)
	}

	ch.send(map[string]any{
		"v":          ProtocolVersion,
		"kind":       "HEY",
		"resumed":    true,
		"session_id": ch.sessionID,
		"replayed":   len(missed),
	})
}

func (ch *channel) handleList(seq int) {
	ch.server.mu.RLock()
	tools := make([]map[string]any, 0, len(ch.server.tools))
	for _, t := range ch.server.tools {
		tools = append(tools, toolMeta(t))
	}
	ch.server.mu.RUnlock()
	ch.send(map[string]any{"v": ProtocolVersion, "kind": "LST", "seq": seq, "tools": tools})
}

func toolMeta(t Tool) map[string]any {
	m := map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"streaming":   t.Streaming,
	}
	if t.Version != "" {
		m["version"] = t.Version
	}
	if t.Input != nil {
		m["input"] = t.Input
	}
	if t.Output != nil {
		m["output"] = t.Output
	}
	if len(t.Effects) > 0 {
		m["effects"] = t.Effects
	}
	if t.Cost != nil {
		m["cost"] = t.Cost
	}
	if t.Capability != "" {
		m["requires_capability"] = t.Capability
	}
	return m
}

func (ch *channel) handleInvoke(seq int, f map[string]any) {
	ctx, cancel := context.WithCancel(context.Background())
	ch.mu.Lock()
	ch.active[seq] = cancel
	ch.mu.Unlock()
	defer func() {
		ch.mu.Lock()
		delete(ch.active, seq)
		ch.mu.Unlock()
		cancel()
	}()

	traceID, _ := f["trace_id"].(string)
	ctx = context.WithValue(ctx, "trace_id", traceID)

	if pipeline, ok := f["pipeline"].([]any); ok && len(pipeline) > 0 {
		ch.runPipeline(ctx, seq, pipeline, traceID)
		return
	}

	toolName, _ := f["tool"].(string)
	input, _ := f["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}
	ch.invokeTool(ctx, seq, toolName, input, traceID)
}

func (ch *channel) lookupTool(name string) (Tool, bool) {
	ch.server.mu.RLock()
	defer ch.server.mu.RUnlock()
	// Try exact match (with version)
	if t, ok := ch.server.tools[name]; ok {
		return t, true
	}
	// If no @ in name, find latest matching prefix
	if !strings.Contains(name, "@") {
		for k, v := range ch.server.tools {
			if strings.HasPrefix(k, name+"@") || k == name {
				return v, true
			}
		}
	}
	return Tool{}, false
}

func (ch *channel) invokeTool(ctx context.Context, seq int, toolName string, input map[string]any, traceID string) (any, error) {
	tool, ok := ch.lookupTool(toolName)
	if !ok {
		ch.sendErr(seq, "TOOL_NOT_FOUND", "tool: "+toolName)
		return nil, fmt.Errorf("not found")
	}

	// Capability check
	if tool.Capability != "" {
		ch.mu.Lock()
		granted := ch.hasCapability(tool.Capability)
		ch.mu.Unlock()
		if !granted {
			ch.sendErr(seq, "MISSING_CAPABILITY",
				"tool requires "+tool.Capability)
			return nil, fmt.Errorf("missing capability")
		}
	}

	// Input validation
	if err := validateInput(input, tool.Input); err != nil {
		ch.sendErr(seq, "INVALID_INPUT", err.Error())
		return nil, err
	}

	// Cost limit check (advisory; per-call check)
	ch.mu.Lock()
	exceeded := ch.maxCostUSD > 0 && ch.costAccum.USD >= ch.maxCostUSD
	ch.mu.Unlock()
	if exceeded {
		ch.sendErr(seq, "COST_LIMIT", "max_cost_usd reached")
		return nil, fmt.Errorf("cost limit")
	}

	if tool.StreamHandler != nil {
		chunks := make(chan any, 32)
		errCh := make(chan error, 1)
		costCh := make(chan *Cost, 1)
		go func() {
			cost, err := tool.StreamHandler(ctx, input, chunks)
			costCh <- cost
			errCh <- err
			close(chunks)
		}()
		for chunk := range chunks {
			frame := map[string]any{
				"v":    ProtocolVersion,
				"kind": "STR",
				"seq":  seq,
				"data": chunk,
			}
			if traceID != "" {
				frame["trace_id"] = traceID
			}
			ch.send(frame)
		}
		cost := <-costCh
		if err := <-errCh; err != nil {
			ch.sendErr(seq, "INTERNAL", err.Error())
			return nil, err
		}
		end := map[string]any{
			"v":    ProtocolVersion,
			"kind": "END",
			"seq":  seq,
		}
		if cost != nil {
			end["cost"] = cost
			ch.accumulateCost(cost)
		}
		if traceID != "" {
			end["trace_id"] = traceID
		}
		ch.send(end)
		return nil, nil
	}

	out, cost, err := tool.Handler(ctx, input)
	if err != nil {
		ch.sendErr(seq, "INTERNAL", err.Error())
		return nil, err
	}
	frame := map[string]any{
		"v":      ProtocolVersion,
		"kind":   "RES",
		"seq":    seq,
		"output": out,
	}
	if cost != nil {
		frame["cost"] = cost
		ch.accumulateCost(cost)
	}
	if traceID != "" {
		frame["trace_id"] = traceID
	}
	ch.send(frame)
	return out, nil
}

func (ch *channel) accumulateCost(c *Cost) {
	if c == nil {
		return
	}
	ch.mu.Lock()
	ch.costAccum.ComputeMs += c.ComputeMs
	ch.costAccum.APICalls += c.APICalls
	ch.costAccum.USD += c.USD
	ch.costAccum.Tokens += c.Tokens
	ch.mu.Unlock()
}

// hasCapability does prefix-and-wildcard matching against grants.
func (ch *channel) hasCapability(required string) bool {
	if ch.grants[required] {
		return true
	}
	// Try wildcard: required="github:read", check grants for "github:*", "github:read:*", etc.
	parts := strings.Split(required, ":")
	for i := 0; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], ":")
		if i < len(parts) {
			if ch.grants[prefix+":*"] {
				return true
			}
		}
		if i == 0 && ch.grants["*"] {
			return true
		}
	}
	return false
}

func (ch *channel) runPipeline(ctx context.Context, seq int, pipeline []any, traceID string) {
	var prev any
	for stageIdx, raw := range pipeline {
		stage, ok := raw.(map[string]any)
		if !ok {
			ch.sendErr(seq, "INVALID_INPUT", fmt.Sprintf("stage %d malformed", stageIdx))
			return
		}
		if toolName, ok := stage["tool"].(string); ok {
			input, _ := stage["input"].(map[string]any)
			if input == nil {
				input = map[string]any{}
			}
			if bind, ok := stage["input_bind"].(map[string]any); ok {
				for k, v := range bind {
					if s, ok := v.(string); ok && s == "$prev" {
						input[k] = prev
					} else {
						input[k] = v
					}
				}
			}
			out, err := ch.invokeToolSilent(ctx, toolName, input)
			if err != nil {
				ch.sendErr(seq, "PIPELINE_STAGE_FAILED",
					fmt.Sprintf("stage %d (%s): %s", stageIdx, toolName, err.Error()))
				return
			}
			prev = out
			continue
		}
		if filterExpr, ok := stage["filter"].(string); ok {
			prev = applyFilterExtended(prev, filterExpr)
			continue
		}
		if mapFields, ok := stage["map"].([]any); ok {
			prev = applyMap(prev, mapFields)
			continue
		}
		ch.sendErr(seq, "INVALID_INPUT", fmt.Sprintf("stage %d: unknown", stageIdx))
		return
	}
	frame := map[string]any{
		"v":      ProtocolVersion,
		"kind":   "RES",
		"seq":    seq,
		"output": prev,
	}
	if traceID != "" {
		frame["trace_id"] = traceID
	}
	ch.send(frame)
}

func (ch *channel) invokeToolSilent(ctx context.Context, name string, input map[string]any) (any, error) {
	tool, ok := ch.lookupTool(name)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	if tool.Handler == nil {
		return nil, fmt.Errorf("streaming tool %s in pipeline", name)
	}
	if tool.Capability != "" {
		ch.mu.Lock()
		granted := ch.hasCapability(tool.Capability)
		ch.mu.Unlock()
		if !granted {
			return nil, fmt.Errorf("missing capability: %s", tool.Capability)
		}
	}
	if err := validateInput(input, tool.Input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	out, cost, err := tool.Handler(ctx, input)
	if cost != nil {
		ch.accumulateCost(cost)
	}
	return out, err
}

func (ch *channel) handleSubscribe(seq int, f map[string]any) {
	topic, _ := f["topic"].(string)
	filter, _ := f["filter"].(map[string]any)
	ch.server.mu.RLock()
	handler, ok := ch.server.topics[topic]
	ch.server.mu.RUnlock()
	if !ok {
		ch.sendErr(seq, "TOPIC_NOT_FOUND", "topic: "+topic)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch.mu.Lock()
	ch.active[seq] = cancel
	ch.mu.Unlock()

	events := make(chan any, 32)
	go func() {
		_ = handler(ctx, filter, events)
		close(events)
	}()
	ch.send(map[string]any{
		"v":              ProtocolVersion,
		"kind":           "RES",
		"seq":            seq,
		"subscriptionId": seq,
	})
	for ev := range events {
		ch.send(map[string]any{"v": ProtocolVersion, "kind": "EVT", "seq": seq, "data": ev})
	}
}

func (ch *channel) handleCancel(seq int) {
	ch.mu.Lock()
	if cancel, ok := ch.active[seq]; ok {
		cancel()
		delete(ch.active, seq)
	}
	ch.mu.Unlock()
}

func (ch *channel) parkSession() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	sess := &session{
		id:         ch.sessionID,
		agentID:    ch.agentID,
		grants:     ch.grants,
		replayBuf:  ch.replayBuf,
		costAccum:  ch.costAccum,
		maxCostUSD: ch.maxCostUSD,
	}
	ch.server.parkSession(sess)
}

// ───────────────────────────────────────────────────────────────
// QCT (Quark Capability Token)
// ───────────────────────────────────────────────────────────────

// CreateQCT mints a signed token.
func CreateQCT(secret []byte, payload *QCTPayload) (string, error) {
	if payload.IssuedAt == 0 {
		payload.IssuedAt = time.Now().Unix()
	}
	if payload.Expiry == 0 {
		return "", errors.New("payload.Expiry required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	signing := "v1." + encoded
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return "qct.v1." + encoded + "." + sig, nil
}

// VerifyQCT verifies signature and time bounds.
func VerifyQCT(token string, secret []byte) (*QCTPayload, error) {
	if len(secret) == 0 {
		return nil, errors.New("server secret not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != "qct" || parts[1] != "v1" {
		return nil, errors.New("malformed QCT")
	}
	encoded := parts[2]
	sig := parts[3]

	signing := "v1." + encoded
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(sig)) {
		return nil, errors.New("signature mismatch")
	}

	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var p QCTPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}
	now := time.Now().Unix()
	if p.NotBefore > 0 && now < p.NotBefore {
		return nil, errors.New("token not yet valid (nbf)")
	}
	if p.Expiry > 0 && now >= p.Expiry {
		return nil, errors.New("token expired")
	}
	return &p, nil
}

// ───────────────────────────────────────────────────────────────
// Input validation (minimal JSON Schema subset)
// ───────────────────────────────────────────────────────────────

func validateInput(input map[string]any, schema map[string]any) error {
	if schema == nil {
		return nil
	}
	// Check required fields
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			key, _ := r.(string)
			if _, exists := input[key]; !exists {
				return fmt.Errorf("missing required field: %s", key)
			}
		}
	}
	// Check property types (shallow)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	for k, v := range input {
		propSpec, ok := props[k].(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := propSpec["type"].(string)
		if expectedType == "" {
			continue
		}
		if !matchesType(v, expectedType) {
			return fmt.Errorf("field %s: expected %s", k, expectedType)
		}
	}
	return nil
}

func matchesType(v any, expected string) bool {
	switch expected {
	case "string":
		_, ok := v.(string)
		return ok
	case "integer":
		switch v.(type) {
		case int, int64, float64:
			return true
		}
		return false
	case "number":
		switch v.(type) {
		case int, int64, float64:
			return true
		}
		return false
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return true
}

// ───────────────────────────────────────────────────────────────
// Filter expression language (minimal)
// ───────────────────────────────────────────────────────────────
// ─── Pipeline helpers (filter/map in filter.go) ───

func numVal(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	}
	return 0
}

func applyMap(input any, fields []any) any {
	arr, ok := input.([]any)
	if !ok {
		return input
	}
	out := []any{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if len(fields) == 1 {
			if k, ok := fields[0].(string); ok {
				if v, ok := obj[k]; ok {
					out = append(out, v)
					continue
				}
			}
		}
		picked := map[string]any{}
		for _, f := range fields {
			if k, ok := f.(string); ok {
				if v, ok := obj[k]; ok {
					picked[k] = v
				}
			}
		}
		out = append(out, picked)
	}
	return out
}

func intVal(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case int64:
		return int(t)
	}
	return 0
}

// ───────────────────────────────────────────────────────────────
// Replay buffer
// ───────────────────────────────────────────────────────────────

type replayBuffer struct {
	mu      sync.Mutex
	entries []replayEntry
	size    int
}

type replayEntry struct {
	seq   int
	frame map[string]any
}

func newReplayBuffer(size int) *replayBuffer {
	return &replayBuffer{size: size, entries: make([]replayEntry, 0, size)}
}

func (b *replayBuffer) add(frame map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	seq := intVal(frame["seq"])
	b.entries = append(b.entries, replayEntry{seq, frame})
	if len(b.entries) > b.size {
		b.entries = b.entries[len(b.entries)-b.size:]
	}
}

func (b *replayBuffer) since(seq int) []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []map[string]any{}
	for _, e := range b.entries {
		if e.seq > seq {
			out = append(out, e.frame)
		}
	}
	return out
}

// ───────────────────────────────────────────────────────────────
// Helpers
// ───────────────────────────────────────────────────────────────

func newSessionID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "ses_" + base64.RawURLEncoding.EncodeToString(b)
}

// NewTraceID returns a fresh W3C trace_id (32 hex chars).
func NewTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewSpanID returns a fresh W3C span_id (16 hex chars).
func NewSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func upper(s string) string {
	out := []rune{}
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			r -= 32
		}
		out = append(out, r)
	}
	return string(out)
}

// ───────────────────────────────────────────────────────────────
// Demo tools
// ───────────────────────────────────────────────────────────────

func RegisterDemoTools(s *Server) {
	s.RegisterTool(Tool{
		Name:        "time.now",
		Description: "Returns the current server time",
		Effects:     []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, *Cost, error) {
			return map[string]any{
				"unix": time.Now().Unix(),
				"iso":  time.Now().UTC().Format(time.RFC3339),
			}, &Cost{ComputeMs: 1}, nil
		},
	})
	s.RegisterTool(Tool{
		Name:        "echo.upper",
		Description: "Returns input text in uppercase",
		Input: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []string{"text"},
		},
		Effects: []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, *Cost, error) {
			text, _ := in["text"].(string)
			return upper(text), &Cost{ComputeMs: 1}, nil
		},
	})
	s.RegisterTool(Tool{
		Name:        "demo.counter",
		Description: "Streams numbers 1..N with 100ms gap",
		Streaming:   true,
		Effects:     []string{"pure"},
		StreamHandler: func(ctx context.Context, in map[string]any, chunks chan<- any) (*Cost, error) {
			n := intVal(in["n"])
			if n <= 0 || n > 100 {
				n = 5
			}
			for i := 1; i <= n; i++ {
				select {
				case <-ctx.Done():
					return &Cost{ComputeMs: int(time.Now().UnixMilli())}, ctx.Err()
				case chunks <- map[string]any{"value": i}:
				}
				time.Sleep(100 * time.Millisecond)
			}
			return &Cost{ComputeMs: n * 100}, nil
		},
	})
	s.RegisterTool(Tool{
		Name:        "demo.fake_repos",
		Description: "Returns fake repos for composition demos",
		Effects:     []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, *Cost, error) {
			return []any{
				map[string]any{"name": "claude-code", "stars": 12000, "owner": "anthropic"},
				map[string]any{"name": "mcp", "stars": 4500, "owner": "anthropic"},
				map[string]any{"name": "small-repo", "stars": 50, "owner": "anthropic"},
				map[string]any{"name": "tiny", "stars": 3, "owner": "anthropic"},
			}, &Cost{ComputeMs: 1}, nil
		},
	})
	s.RegisterTopic("demo.heartbeat", func(ctx context.Context, filter map[string]any, events chan<- any) error {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case t := <-ticker.C:
				events <- map[string]any{"ts": t.Unix()}
			}
		}
	})
}

// stub to avoid io unused
var _ = io.EOF
