package sael

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ─── SCT ───

func TestQCTRoundTrip(t *testing.T) {
	secret := []byte("test-secret-32bytes-min-recommend")
	payload := &QCTPayload{
		Issuer:     "https://test.example",
		Subject:    "user@example.com",
		Expiry:     time.Now().Add(time.Hour).Unix(),
		Scope:      []string{"github:read:*", "echo:invoke"},
		ClientID:   "my-client",
		MaxCostUSD: 1.50,
	}

	token, err := CreateQCT(secret, payload)
	if err != nil {
		t.Fatalf("CreateQCT: %v", err)
	}
	if !strings.HasPrefix(token, "qct.v1.") {
		t.Fatalf("token format wrong: %s", token)
	}

	verified, err := VerifyQCT(token, secret)
	if err != nil {
		t.Fatalf("VerifyQCT: %v", err)
	}
	if verified.Subject != "user@example.com" {
		t.Errorf("subject mismatch: %s", verified.Subject)
	}
	if len(verified.Scope) != 2 {
		t.Errorf("scope len mismatch: %d", len(verified.Scope))
	}
	if verified.MaxCostUSD != 1.50 {
		t.Errorf("max_cost_usd mismatch: %f", verified.MaxCostUSD)
	}
}

func TestQCTSignatureMismatch(t *testing.T) {
	secret := []byte("real-secret")
	payload := &QCTPayload{
		Issuer:  "test",
		Subject: "u",
		Expiry:  time.Now().Add(time.Hour).Unix(),
		Scope:   []string{"x"},
	}
	token, _ := CreateQCT(secret, payload)
	if _, err := VerifyQCT(token, []byte("wrong-secret")); err == nil {
		t.Fatal("expected signature mismatch error")
	}
}

func TestQCTExpired(t *testing.T) {
	secret := []byte("test")
	payload := &QCTPayload{
		Issuer:  "test",
		Subject: "u",
		Expiry:  time.Now().Add(-time.Hour).Unix(), // already expired
		Scope:   []string{"x"},
	}
	token, _ := CreateQCT(secret, payload)
	_, err := VerifyQCT(token, secret)
	if err == nil {
		t.Fatal("expected expired error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQCTNotBefore(t *testing.T) {
	secret := []byte("test")
	payload := &QCTPayload{
		Issuer:    "test",
		Subject:   "u",
		NotBefore: time.Now().Add(time.Hour).Unix(), // future
		Expiry:    time.Now().Add(2 * time.Hour).Unix(),
		Scope:     []string{"x"},
	}
	token, _ := CreateQCT(secret, payload)
	_, err := VerifyQCT(token, secret)
	if err == nil {
		t.Fatal("expected nbf error")
	}
}

// ─── Tool registration & versioning ───

func TestToolVersioning(t *testing.T) {
	s := NewServer(&ServerOptions{AllowAnonymous: true})

	s.RegisterTool(Tool{
		Name:    "demo.hello",
		Version: "v1",
		Handler: func(ctx context.Context, in map[string]any) (any, *Cost, error) {
			return "v1 result", &Cost{ComputeMs: 1}, nil
		},
	})
	s.RegisterTool(Tool{
		Name:    "demo.hello",
		Version: "v2",
		Handler: func(ctx context.Context, in map[string]any) (any, *Cost, error) {
			return "v2 result", &Cost{ComputeMs: 1}, nil
		},
	})

	// Both versions registered
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.tools["demo.hello@v1"]; !ok {
		t.Fatal("v1 not registered")
	}
	if _, ok := s.tools["demo.hello@v2"]; !ok {
		t.Fatal("v2 not registered")
	}
}

// ─── Input validation ───

func TestValidateInputMissingRequired(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []any{"text"},
	}
	err := validateInput(map[string]any{}, schema)
	if err == nil {
		t.Fatal("expected missing required error")
	}
	if !strings.Contains(err.Error(), "text") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInputTypeMismatch(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	err := validateInput(map[string]any{"count": "not a number"}, schema)
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestValidateInputAcceptsValid(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text":    map[string]any{"type": "string"},
			"count":   map[string]any{"type": "integer"},
			"enabled": map[string]any{"type": "boolean"},
		},
		"required": []any{"text"},
	}
	err := validateInput(map[string]any{
		"text":    "hello",
		"count":   42,
		"enabled": true,
	}, schema)
	if err != nil {
		t.Errorf("expected no error: %v", err)
	}
}

// ─── Filter expression language ───

func TestFilterComparison(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "stars": 50.0},
		map[string]any{"name": "b", "stars": 200.0},
		map[string]any{"name": "c", "stars": 1000.0},
	}
	arr := ApplyFilter(items, "stars > 100")
	if len(arr) != 2 {
		t.Fatalf("expected 2 items, got %d", len(arr))
	}
}

func TestFilterAnd(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "stars": 50.0, "owner": "x"},
		map[string]any{"name": "b", "stars": 200.0, "owner": "x"},
		map[string]any{"name": "c", "stars": 200.0, "owner": "y"},
	}
	arr := ApplyFilter(items, "stars > 100 && owner == x")
	if len(arr) != 1 {
		t.Fatalf("expected 1, got %d", len(arr))
	}
}

func TestFilterContains(t *testing.T) {
	items := []any{
		map[string]any{"name": "claude-code"},
		map[string]any{"name": "mcp"},
		map[string]any{"name": "claude-desktop"},
	}
	arr := ApplyFilter(items, "name contains claude")
	if len(arr) != 2 {
		t.Fatalf("expected 2 (claude-*), got %d", len(arr))
	}
}

func TestFilterStartsWith(t *testing.T) {
	items := []any{
		map[string]any{"name": "AI Assistant"},
		map[string]any{"name": "Database Tools"},
		map[string]any{"name": "AI Research"},
	}
	arr := ApplyFilter(items, "name startsWith AI")
	if len(arr) != 2 {
		t.Fatalf("expected 2 (AI *), got %d", len(arr))
	}
}

// ─── Map projection ───

func TestMapSingleField(t *testing.T) {
	items := []any{
		map[string]any{"name": "claude-code", "stars": 12000},
		map[string]any{"name": "mcp", "stars": 4500},
	}
	result := applyMap(items, []any{"name"})
	arr, _ := result.([]any)
	if len(arr) != 2 || arr[0] != "claude-code" {
		t.Fatalf("unexpected: %v", arr)
	}
}

func TestMapMultipleFields(t *testing.T) {
	items := []any{
		map[string]any{"name": "x", "stars": 100, "extra": "ignored"},
	}
	result := applyMap(items, []any{"name", "stars"})
	arr, _ := result.([]any)
	obj, _ := arr[0].(map[string]any)
	if _, ok := obj["extra"]; ok {
		t.Fatal("extra should be filtered out")
	}
	if obj["name"] != "x" || obj["stars"] != 100 {
		t.Fatalf("unexpected: %v", obj)
	}
}

// ─── Capability matching ───

func TestCapabilityExactMatch(t *testing.T) {
	ch := &channel{grants: map[string]bool{"github:read": true}}
	if !ch.hasCapability("github:read") {
		t.Fatal("exact match should succeed")
	}
}

func TestCapabilityWildcardDescendant(t *testing.T) {
	ch := &channel{grants: map[string]bool{"github:read:*": true}}
	if !ch.hasCapability("github:read:repo:foo") {
		t.Fatal("wildcard descendant should match")
	}
}

func TestCapabilityGlobalWildcard(t *testing.T) {
	ch := &channel{grants: map[string]bool{"*": true}}
	if !ch.hasCapability("anything:goes:here") {
		t.Fatal("global wildcard should match")
	}
}

func TestCapabilityNoMatch(t *testing.T) {
	ch := &channel{grants: map[string]bool{"slack:notify:*": true}}
	if ch.hasCapability("github:write") {
		t.Fatal("unrelated cap should not match")
	}
}

// ─── Replay buffer ───

func TestReplayBuffer(t *testing.T) {
	buf := newReplayBuffer(3)
	buf.add(map[string]any{"seq": 1, "kind": "RES"})
	buf.add(map[string]any{"seq": 2, "kind": "RES"})
	buf.add(map[string]any{"seq": 3, "kind": "RES"})
	buf.add(map[string]any{"seq": 4, "kind": "RES"}) // evicts seq=1

	since := buf.since(1)
	if len(since) != 3 {
		t.Fatalf("expected 3 entries with seq > 1, got %d", len(since))
	}
}

func TestReplayBufferSinceSeq(t *testing.T) {
	buf := newReplayBuffer(10)
	for i := 1; i <= 5; i++ {
		buf.add(map[string]any{"seq": i, "kind": "RES"})
	}
	since := buf.since(3)
	if len(since) != 2 { // seq 4, 5
		t.Fatalf("expected 2, got %d", len(since))
	}
}

// ─── Helpers ───

func TestNewSessionID(t *testing.T) {
	id1 := newSessionID()
	id2 := newSessionID()
	if id1 == id2 {
		t.Fatal("session IDs should be unique")
	}
	if !strings.HasPrefix(id1, "ses_") {
		t.Fatalf("session ID format: %s", id1)
	}
}

func TestNewTraceID(t *testing.T) {
	id := NewTraceID()
	if len(id) != 32 {
		t.Fatalf("trace_id should be 32 hex chars, got %d", len(id))
	}
}

func TestNewSpanID(t *testing.T) {
	id := NewSpanID()
	if len(id) != 16 {
		t.Fatalf("span_id should be 16 hex chars, got %d", len(id))
	}
}

// ─── v1.0: Extended filter language ───

func TestFilterParens(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "stars": 200.0, "verified": true},
		map[string]any{"name": "b", "stars": 50.0, "verified": false},
		map[string]any{"name": "c", "stars": 200.0, "verified": false},
		map[string]any{"name": "d", "stars": 50.0, "verified": true},
	}
	// (stars > 100 || verified) && name != "d"
	result := ApplyFilter(items, "(stars > 100 || verified == true) && name != \"d\"")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(result), result)
	}
}

func TestFilterNotOperator(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "archived": true},
		map[string]any{"name": "b", "archived": false},
	}
	result := ApplyFilter(items, "!archived")
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

func TestFilterIn(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "lang": "go"},
		map[string]any{"name": "b", "lang": "rust"},
		map[string]any{"name": "c", "lang": "python"},
	}
	result := ApplyFilter(items, "lang in [\"go\", \"rust\"]")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestFilterNotIn(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "status": "active"},
		map[string]any{"name": "b", "status": "archived"},
		map[string]any{"name": "c", "status": "deleted"},
	}
	result := ApplyFilter(items, "status notIn [\"archived\", \"deleted\"]")
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

func TestFilterMatches(t *testing.T) {
	items := []any{
		map[string]any{"email": "a@example.com"},
		map[string]any{"email": "b@other.com"},
		map[string]any{"email": "c@example.com"},
	}
	result := ApplyFilter(items, `email matches ".*example.*"`)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestFilterNestedField(t *testing.T) {
	items := []any{
		map[string]any{"name": "a", "meta": map[string]any{"score": 50.0}},
		map[string]any{"name": "b", "meta": map[string]any{"score": 200.0}},
	}
	result := ApplyFilter(items, "meta.score > 100")
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

func TestFilterArithmeticValue(t *testing.T) {
	items := []any{
		map[string]any{"a": 10.0, "b": 5.0},
		map[string]any{"a": 3.0, "b": 5.0},
	}
	result := ApplyFilter(items, "a > b * 1.5")
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
}

// ─── v1.0: Federation ───

func TestFederationRegister(t *testing.T) {
	f := NewFederation()
	f.Register(FederationRoute{Host: "github-tools.example.com", URL: "wss://github-tools.example.com/sael/ws"})
	hosts := f.Hosts()
	if len(hosts) != 1 || hosts[0] != "github-tools.example.com" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
}

func TestFederationLookup(t *testing.T) {
	f := NewFederation()
	f.Register(FederationRoute{Host: "x", URL: "wss://x/sael/ws"})
	if _, ok := f.Lookup("x"); !ok {
		t.Fatal("Lookup should find registered host")
	}
	if _, ok := f.Lookup("missing"); ok {
		t.Fatal("Lookup should not find unregistered host")
	}
}

func TestFederationForward(t *testing.T) {
	f := NewFederation()
	f.Register(FederationRoute{Host: "h", URL: "wss://h/sael/ws"})
	frame := map[string]any{"kind": "INV", "tool": "x.test"}
	resp, err := f.Forward(context.Background(), "h", "tok", frame)
	if err != nil {
		t.Fatal(err)
	}
	if resp["via"] != "h" {
		t.Fatalf("expected via=h, got %v", resp["via"])
	}
}

// ─── v1.0: MessagePack ───

func TestMessagePackRoundTrip(t *testing.T) {
	frame := map[string]any{
		"v":    int64(1),
		"kind": "RES",
		"seq":  int64(5),
		"output": map[string]any{
			"name":  "test",
			"stars": int64(100),
		},
	}
	encoded, err := MarshalFrameMsgpack(frame)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalFrameMsgpack(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded["kind"] != "RES" {
		t.Errorf("kind mismatch: %v", decoded["kind"])
	}
	if decoded["seq"].(int64) != 5 {
		t.Errorf("seq mismatch: %v", decoded["seq"])
	}
}

func TestSubprotocolNames(t *testing.T) {
	if MsgpackSubprotocol != "application/x-sael-msgpack" {
		t.Errorf("msgpack subprotocol: %s", MsgpackSubprotocol)
	}
	if JSONSubprotocol != "application/x-sael-json" {
		t.Errorf("json subprotocol: %s", JSONSubprotocol)
	}
}
