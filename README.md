# Quark Protocol

> Streaming-first AI tool protocol. Successor to MCP.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Site](https://img.shields.io/badge/site-unyly.org%2Fquark-ff5cf1)](https://unyly.org/quark)

Quark replaces Model Context Protocol (MCP) in production scenarios where MCP cracks: streaming, composition, subscriptions, backpressure, capability security, and multi-agent — all built into the protocol, not bolted on.

## Why Quark?

- **10× faster** in real workloads (server-side pipeline composition)
- **Streaming-native** — every tool can stream, with built-in backpressure
- **Stateful sessions** — keep context across tool calls, save tokens
- **Subscriptions** — first-class push events, no polling
- **Capability-based security** — agents can't do what users didn't allow
- **Multi-agent** — agents can call each other directly
- **MCP-compatible** — adapter wraps existing MCP servers, zero migration

## Quick links

- **Spec v0.1:** [`docs/spec.md`](./docs/spec.md)
- **Landing & live demo:** [unyly.org/quark](https://unyly.org/quark)
- **Online spec viewer:** [unyly.org/quark/spec](https://unyly.org/quark/spec)
- **TypeScript SDK:** [`clients/typescript/`](./clients/typescript) — `@unyly/quark-client`
- **Go server SDK:** [`clients/go/`](./clients/go)

## Quick start (TypeScript)

```bash
pnpm add @unyly/quark-client
```

```ts
import { Quark } from '@unyly/quark-client'

const ch = await Quark.connect('wss://unyly.org/quark/ws', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
})

const repos = await ch.invoke('demo.fake_repos')

// Server-side pipeline composition (killer feature)
const filtered = await ch.pipeline([
  { tool: 'demo.fake_repos' },
  { filter: 'stars > 100' },
  { map: ['name'] },
])
// → ['claude-code', 'mcp']
```

## Quick start (Go server)

```go
import "github.com/FasadSalatov/quark/clients/go"

srv := quark.NewServer()
srv.RegisterTool(quark.Tool{
    Name: "echo.upper",
    Description: "Echo text in uppercase",
    Handler: func(ctx context.Context, in map[string]any) (any, error) {
        return strings.ToUpper(in["text"].(string)), nil
    },
})
http.Handle("/quark/ws", srv)
```

## Live demo

Open [unyly.org/quark](https://unyly.org/quark) → press **«connect»** to see live WebSocket frames against a reference server. Try LST, INV, streaming, pipeline composition, subscriptions.

## Project status

**v0.1 — Draft (2026-06-05).** Reference Go server and TypeScript client are production-ready as-is. Spec is open for community feedback before v1.0.

See [Roadmap section in spec](./docs/spec.md#16-roadmap).

## Contributing

- Open issues for spec ambiguities or impl bugs
- PRs for additional client SDKs (Python, Rust, Swift) are welcome
- Discussion: [unyly.org/quark](https://unyly.org/quark) or [@Fasad_Salatov](https://t.me/Fasad_Salatov)

## License

MIT — see [LICENSE](./LICENSE).

## Built by

[Unyly](https://unyly.org) — MCP marketplace by [Fasad Salatov](https://github.com/FasadSalatov).
