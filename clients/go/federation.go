package quark

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// FederationRoute describes a downstream Quark server that this server can route to.
type FederationRoute struct {
	// Host is the target server identifier (e.g. "github-tools.example.com").
	Host string
	// URL is the WebSocket URL to dial (e.g. "wss://github-tools.example.com/quark/ws").
	URL string
	// Token is an optional QCT used for server-to-server auth. Falls back to
	// forwarding the client's QCT if empty.
	Token string
}

// Federation manages downstream Quark connections.
type Federation struct {
	routes map[string]FederationRoute
	mu     sync.RWMutex
}

// NewFederation returns a fresh federation registry.
func NewFederation() *Federation {
	return &Federation{routes: map[string]FederationRoute{}}
}

// Register adds a downstream route.
func (f *Federation) Register(route FederationRoute) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[route.Host] = route
}

// Hosts returns the list of all federated server hosts (for HEY response).
func (f *Federation) Hosts() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.routes))
	for h := range f.routes {
		out = append(out, h)
	}
	return out
}

// Lookup returns the route for a host, or false.
func (f *Federation) Lookup(host string) (FederationRoute, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	r, ok := f.routes[host]
	return r, ok
}

// Forward proxies an INV frame to a downstream server.
//
// This is a minimal in-memory placeholder; a production implementation would
// pool WebSocket connections to downstreams. v1.0 ships the routing surface;
// pooling/circuit-breakers come in v1.1.
func (f *Federation) Forward(
	ctx context.Context,
	host string,
	clientToken string,
	frame map[string]any,
) (map[string]any, error) {
	route, ok := f.Lookup(host)
	if !ok {
		return nil, fmt.Errorf("federation: host %s not registered", host)
	}
	_ = route // placeholder: dial route.URL, send HEY+frame, return RES
	// Echo with a federation marker for now.
	out := map[string]any{}
	for k, v := range frame {
		out[k] = v
	}
	out["via"] = host
	out["v"] = ProtocolVersion
	out["kind"] = "RES"
	out["output"] = map[string]any{
		"_note": "federation forwarding scaffolded — implement Dial in v1.1",
		"frame": frame,
	}
	if clientToken != "" {
		out["forwarded_token"] = "(client token, hash="+ shortHash(clientToken) + ")"
	}
	return out, nil
}

func shortHash(s string) string {
	if len(s) < 16 {
		return s
	}
	return s[:8] + "…" + s[len(s)-4:]
}

// MarshalRoutes for tests and snapshots.
func (f *Federation) MarshalRoutes() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return json.Marshal(f.routes)
}
