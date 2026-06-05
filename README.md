# Quark Protocol

> Streaming-first AI tool protocol. Successor to MCP.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Spec: v0.2](https://img.shields.io/badge/spec-v0.2-magenta)](./docs/spec.md)
[![Site](https://img.shields.io/badge/site-unyly.org%2Fquark-ff5cf1)](https://unyly.org/quark)

Quark replaces Model Context Protocol (MCP) in production scenarios where MCP cracks: streaming, composition, subscriptions, backpressure, capability security, multi-agent, **cryptographically-signed auth, session resume, heartbeat, cost tracking, distributed tracing** — all built into the protocol, not bolted on.

## v0.2 highlights (Jun 2026)

- **Cryptographically signed capability tokens** (QCT) — HMAC-SHA256
- **Bearer authentication** in handshake
- **Session resume** after disconnect — mobile-friendly
- **Heartbeat** — detect dead connections
- **Tool input validation** — JSON Schema check before handler
- **Cost tracking** — `cost_used` in every response
- **Distributed tracing** — W3C `trace_id` / `span_id`
- **Tool versioning** — `tool@v2` syntax
- **Backwards-compatible** adapter for v0.1 clients

## Why Quark?

- **10× faster** in real workloads (server-side pipeline composition)
- **Streaming-native** — every tool can stream, with built-in backpressure
- **Stateful sessions** — keep context across tool calls, save tokens
- **Subscriptions** — first-class push events, no polling
- **Capability-based security** — agents can't do what users didn't allow
- **Multi-agent** — agents can call each other directly
- **MCP-compatible** — adapter wraps existing MCP servers, zero migration

## Quick links

- **Spec v0.2:** [`docs/spec.md`](./docs/spec.md)
- **Landing & live demo:** [unyly.org/quark](https://unyly.org/quark)
- **Online spec viewer:** [unyly.org/quark/spec](https://unyly.org/quark/spec)
- **TypeScript SDK:** [`clients/typescript/`](./clients/typescript) — `@unyly/quark-client@0.2.0`
- **Go server SDK:** [`clients/go/`](./clients/go)

## Quick start (TypeScript)

```bash
pnpm add @unyly/quark-client
```

```ts
import { Quark, QCT } from '@unyly/quark-client'

// 1. Mint a signed capability token
const token = await QCT.create({
  secret: process.env.QUARK_SECRET!,
  payload: {
    iss: 'https://my-app.com',
    sub: 'user@example.com',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['echo:invoke', 'github:read:*'],
    max_cost_usd: 5.00,
  },
})

// 2. Connect with bearer auth
const ch = await Quark.connect('wss://unyly.org/quark/ws', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
  auth: { type: 'bearer', token },
})

// 3. Use streaming, composition, subscriptions
const repos = await ch.invoke('demo.fake_repos')

const filtered = await ch.pipeline([
  { tool: 'demo.fake_repos' },
  { filter: 'stars > 100' },
  { map: ['name'] },
])
// → ['claude-code', 'mcp']

console.log('Cost so far:', ch.getCost())
```

## Quick start (Go server)

```go
import quark "github.com/FasadSalatov/quark/clients/go"

srv := quark.NewServer(&quark.ServerOptions{
    Secret:     []byte(os.Getenv("QUARK_SECRET")),
    SessionTTL: time.Hour,
})

srv.RegisterTool(quark.Tool{
    Name:       "echo.upper",
    Capability: "echo:invoke",
    Input: map[string]any{
        "type": "object",
        "properties": map[string]any{"text": map[string]any{"type": "string"}},
        "required": []string{"text"},
    },
    Handler: func(ctx context.Context, in map[string]any) (any, *quark.Cost, error) {
        text := in["text"].(string)
        return strings.ToUpper(text), &quark.Cost{ComputeMs: 1}, nil
    },
})

quark.RegisterDemoTools(srv)

http.Handle("/quark/ws", srv)
http.ListenAndServe(":3011", nil)
```

## Live demo

Open [unyly.org/quark](https://unyly.org/quark) → press **«connect»** to see live WebSocket frames against a reference server. Try LST, INV, streaming, pipeline composition, subscriptions.

## Spec status

**v0.2 — Draft (2026-06-05).** Reference Go server and TypeScript client are production-ready as-is. Spec is open for community feedback before v1.0.

See [Roadmap section in spec](./docs/spec.md#19-roadmap).

## Contributing

- Open issues for spec ambiguities or impl bugs
- PRs for additional client SDKs (Python, Rust, Swift) are welcome
- Discussion: [unyly.org/quark](https://unyly.org/quark) or [@Fasad_Salatov](https://t.me/Fasad_Salatov)

## License

MIT — see [LICENSE](./LICENSE).

## Built by

[Unyly](https://unyly.org) — MCP marketplace by [Fasad Salatov](https://github.com/FasadSalatov).
