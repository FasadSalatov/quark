// Package quark — reference Go implementation of the Quark Protocol v0.1.
//
// Spec: docs/quark/spec.md or https://unyly.org/quark
//
// Quark is a streaming-first AI tool protocol that replaces MCP.
// Features: streaming, server-side pipeline composition, subscriptions,
// backpressure, capability-based security, MCP compatibility layer.
package quark

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const ProtocolVersion = 1

type Tool struct {
	Name        string
	Description string
	Input       map[string]any
	Output      map[string]any
	Effects     []string
	Cost        *Cost
	Streaming   bool
	Capability  string

	Handler       func(ctx context.Context, input map[string]any) (any, error)
	StreamHandler func(ctx context.Context, input map[string]any, chunks chan<- any) error
}

type Cost struct {
	Estimate float64 `json:"estimate"`
	Currency string  `json:"currency"`
}

type TopicHandler func(ctx context.Context, filter map[string]any, events chan<- any) error

type Server struct {
	tools    map[string]Tool
	topics   map[string]TopicHandler
	upgrader websocket.Upgrader
	mu       sync.RWMutex
}

func NewServer() *Server {
	return &Server{
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
	s.mu.Lock()
	s.tools[t.Name] = t
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
		server: s,
		conn:   conn,
		writes: make(chan map[string]any, 64),
		active: map[int]context.CancelFunc{},
		grants: map[string]bool{},
	}
	go ch.writer()
	ch.reader()
}

type channel struct {
	server  *Server
	conn    *websocket.Conn
	writes  chan map[string]any
	mu      sync.Mutex
	active  map[int]context.CancelFunc
	grants  map[string]bool
	agentID string
}

func (ch *channel) send(payload map[string]any) {
	select {
	case ch.writes <- payload:
	default:
	}
}

func (ch *channel) sendErr(seq int, code, msg string) {
	ch.send(map[string]any{"kind": "ERR", "seq": seq, "code": code, "message": msg})
}

func (ch *channel) writer() {
	for f := range ch.writes {
		b, _ := json.Marshal(f)
		if err := ch.conn.WriteMessage(websocket.TextMessage, b); err != nil {
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

		switch kind {
		case "HEY":
			ch.handleHey(f)
		case "LST":
			ch.handleList(seq)
		case "INV":
			go ch.handleInvoke(seq, f)
		case "SUB":
			go ch.handleSubscribe(seq, f)
		case "UNS", "CAN":
			ch.handleCancel(seq)
		case "BYE":
			return
		default:
			ch.sendErr(seq, "UNKNOWN_KIND", "unknown frame: "+kind)
		}
	}
}

func (ch *channel) handleHey(f map[string]any) {
	if agent, ok := f["agent"].(map[string]any); ok {
		if id, ok := agent["id"].(string); ok {
			ch.agentID = id
		}
	}
	if caps, ok := f["capabilities"].([]any); ok {
		ch.mu.Lock()
		for _, c := range caps {
			if s, ok := c.(string); ok {
				ch.grants[s] = true
			}
		}
		ch.mu.Unlock()
	}
	ch.server.mu.RLock()
	toolCount := len(ch.server.tools)
	topicCount := len(ch.server.topics)
	ch.server.mu.RUnlock()
	ch.send(map[string]any{
		"kind": "HEY",
		"v":    ProtocolVersion,
		"server": map[string]any{
			"id":      "unyly-quark-ref",
			"name":    "Unyly Quark Reference Server",
			"version": "0.1.0",
		},
		"supports": []string{"streaming", "subscribe", "compose", "capabilities"},
		"tools":    toolCount,
		"topics":   topicCount,
	})
}

func (ch *channel) handleList(seq int) {
	ch.server.mu.RLock()
	tools := make([]map[string]any, 0, len(ch.server.tools))
	for _, t := range ch.server.tools {
		tools = append(tools, toolMeta(t))
	}
	ch.server.mu.RUnlock()
	ch.send(map[string]any{"kind": "LST", "seq": seq, "tools": tools})
}

func toolMeta(t Tool) map[string]any {
	m := map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"streaming":   t.Streaming,
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

	if pipeline, ok := f["pipeline"].([]any); ok && len(pipeline) > 0 {
		ch.runPipeline(ctx, seq, pipeline)
		return
	}

	toolName, _ := f["tool"].(string)
	input, _ := f["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}
	ch.invokeTool(ctx, seq, toolName, input)
}

func (ch *channel) invokeTool(ctx context.Context, seq int, toolName string, input map[string]any) (any, error) {
	ch.server.mu.RLock()
	tool, ok := ch.server.tools[toolName]
	ch.server.mu.RUnlock()
	if !ok {
		ch.sendErr(seq, "TOOL_NOT_FOUND", "tool: "+toolName)
		return nil, fmt.Errorf("not found")
	}
	if tool.Capability != "" {
		ch.mu.Lock()
		granted := ch.grants[tool.Capability]
		ch.mu.Unlock()
		if !granted {
			ch.sendErr(seq, "MISSING_CAPABILITY",
				"tool "+toolName+" requires "+tool.Capability)
			return nil, fmt.Errorf("missing capability")
		}
	}

	if tool.StreamHandler != nil {
		chunks := make(chan any, 32)
		errCh := make(chan error, 1)
		go func() {
			errCh <- tool.StreamHandler(ctx, input, chunks)
			close(chunks)
		}()
		for chunk := range chunks {
			ch.send(map[string]any{"kind": "STR", "seq": seq, "data": chunk})
		}
		if err := <-errCh; err != nil {
			ch.sendErr(seq, "INTERNAL", err.Error())
			return nil, err
		}
		ch.send(map[string]any{"kind": "END", "seq": seq})
		return nil, nil
	}

	out, err := tool.Handler(ctx, input)
	if err != nil {
		ch.sendErr(seq, "INTERNAL", err.Error())
		return nil, err
	}
	ch.send(map[string]any{"kind": "RES", "seq": seq, "output": out})
	return out, nil
}

func (ch *channel) runPipeline(ctx context.Context, seq int, pipeline []any) {
	var prev any
	for stageIdx, raw := range pipeline {
		stage, ok := raw.(map[string]any)
		if !ok {
			ch.sendErr(seq, "INVALID_INPUT", fmt.Sprintf("stage %d: bad", stageIdx))
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
			prev = applyFilter(prev, filterExpr)
			continue
		}
		if mapFields, ok := stage["map"].([]any); ok {
			prev = applyMap(prev, mapFields)
			continue
		}
		ch.sendErr(seq, "INVALID_INPUT", fmt.Sprintf("stage %d: unknown", stageIdx))
		return
	}
	ch.send(map[string]any{"kind": "RES", "seq": seq, "output": prev})
}

func (ch *channel) invokeToolSilent(ctx context.Context, name string, input map[string]any) (any, error) {
	ch.server.mu.RLock()
	tool, ok := ch.server.tools[name]
	ch.server.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	if tool.Handler == nil {
		return nil, fmt.Errorf("streaming-only tool %s in pipeline", name)
	}
	if tool.Capability != "" {
		ch.mu.Lock()
		granted := ch.grants[tool.Capability]
		ch.mu.Unlock()
		if !granted {
			return nil, fmt.Errorf("missing capability: %s", tool.Capability)
		}
	}
	return tool.Handler(ctx, input)
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
	ch.send(map[string]any{"kind": "RES", "seq": seq, "subscriptionId": seq})
	for ev := range events {
		ch.send(map[string]any{"kind": "EVT", "seq": seq, "data": ev})
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

// ─── Pipeline helpers ───
func applyFilter(input any, expr string) any {
	arr, ok := input.([]any)
	if !ok {
		return input
	}
	out := []any{}
	for _, item := range arr {
		if obj, ok := item.(map[string]any); ok && evalFilter(obj, expr) {
			out = append(out, obj)
		}
	}
	return out
}
func evalFilter(obj map[string]any, expr string) bool {
	tokens := tokenize(expr)
	if len(tokens) != 3 {
		return true
	}
	field, op, val := tokens[0], tokens[1], tokens[2]
	got, ok := obj[field]
	if !ok {
		return false
	}
	switch op {
	case ">":
		return numVal(got) > numVal(val)
	case "<":
		return numVal(got) < numVal(val)
	case ">=":
		return numVal(got) >= numVal(val)
	case "<=":
		return numVal(got) <= numVal(val)
	case "==", "=":
		return fmt.Sprintf("%v", got) == val
	}
	return false
}
func tokenize(s string) []string {
	out := []string{}
	cur := ""
	for _, ch := range s {
		if ch == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(ch)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
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

// ─── Demo tools ───
func RegisterDemoTools(s *Server) {
	s.RegisterTool(Tool{
		Name: "time.now", Description: "Returns the current server time",
		Effects: []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, error) {
			return map[string]any{"unix": time.Now().Unix(), "iso": time.Now().UTC().Format(time.RFC3339)}, nil
		},
	})
	s.RegisterTool(Tool{
		Name: "echo.upper", Description: "Returns input text in uppercase",
		Effects: []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, error) {
			text, _ := in["text"].(string)
			return upper(text), nil
		},
	})
	s.RegisterTool(Tool{
		Name: "demo.counter", Description: "Streams numbers 1..N with 100ms gap",
		Streaming: true, Effects: []string{"pure"},
		StreamHandler: func(ctx context.Context, in map[string]any, chunks chan<- any) error {
			n := intVal(in["n"])
			if n <= 0 || n > 100 {
				n = 5
			}
			for i := 1; i <= n; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case chunks <- map[string]any{"value": i}:
				}
				time.Sleep(100 * time.Millisecond)
			}
			return nil
		},
	})
	s.RegisterTool(Tool{
		Name: "demo.fake_repos", Description: "Returns fake repos for composition demos",
		Effects: []string{"pure"},
		Handler: func(ctx context.Context, in map[string]any) (any, error) {
			return []any{
				map[string]any{"name": "claude-code", "stars": 12000, "owner": "anthropic"},
				map[string]any{"name": "mcp", "stars": 4500, "owner": "anthropic"},
				map[string]any{"name": "small-repo", "stars": 50, "owner": "anthropic"},
				map[string]any{"name": "tiny", "stars": 3, "owner": "anthropic"},
			}, nil
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
