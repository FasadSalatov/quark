# Quark Protocol

> Streaming-first AI tool protocol. Successor to MCP. **Stable v1.0.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Spec: v1.0 stable](https://img.shields.io/badge/spec-v1.0%20stable-magenta)](./docs/spec.md)
[![Site](https://img.shields.io/badge/site-unyly.org%2Fquark-ff5cf1)](https://unyly.org/quark)
[![Tests: 78](https://img.shields.io/badge/tests-78%20green-lime)]()

Quark replaces Model Context Protocol (MCP) in production scenarios where MCP cracks: streaming, composition, subscriptions, backpressure, capability security, multi-agent, signed auth, session resume, heartbeat, cost tracking, distributed tracing, federation, MessagePack — all built into the protocol, not bolted on.

## v1.0 — stable (Jun 2026)

- **Stability guarantee** — no breaking changes through v1.x (12-month deprecation window for v2.0)
- **Federation** — server-to-server routing via mesh discovery
- **MessagePack binary frames** — opt-in via subprotocol negotiation
- **Extended filter language** — `matches`, `in`, `notIn`, parens, `!`, arithmetic, nested fields
- **Schema registry** — `$ref` to `https://schemas.quark.dev/` standard types
- **Conformance test suite** — Go ↔ TS ↔ Python cross-implementation tests

Plus everything from v0.2: signed capability tokens (QCT), bearer auth, session resume, heartbeat, input validation, cost tracking, W3C tracing, tool versioning.

## Why Quark?

- **10× faster** in real workloads (server-side pipeline composition)
- **Streaming-native** — every tool can stream, backpressure built-in
- **Stateful** — sessions resume after disconnect, contexts preserved
- **Subscriptions** — first-class push events
- **Secure** — signed capability tokens, audit trails
- **Observable** — cost + tracing in every frame
- **Federated** — servers route invocations to specialized backends
- **MCP-compatible** — adapter wraps existing MCP servers, zero migration

## Quick links

- **Spec v1.0** (~600 lines, MIT): [`docs/spec.md`](./docs/spec.md)
- **Landing & live demo:** [unyly.org/quark](https://unyly.org/quark)
- **Online spec viewer:** [unyly.org/quark/spec](https://unyly.org/quark/spec)
- **TypeScript SDK** (`@unyly/quark-client@1.0.0`): [`clients/typescript/`](./clients/typescript)
- **Go server SDK**: [`clients/go/`](./clients/go)
- **Python SDK** (`quark-client@1.0.0`): [`clients/python/`](./clients/python)

## Quick start

### TypeScript

```bash
pnpm add @unyly/quark-client
```

```ts
import { Quark, QCT } from '@unyly/quark-client'

const token = await QCT.create({
  secret: process.env.QUARK_SECRET!,
  payload: {
    iss: 'https://my-app.com',
    sub: 'user@example.com',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['echo:invoke'],
  },
})

const ch = await Quark.connect('wss://unyly.org/quark/ws', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
  auth: { type: 'bearer', token },
})

// One-shot
const result = await ch.invoke('echo.upper', { text: 'hello' })

// Pipeline composition
const filtered = await ch.pipeline([
  { tool: 'demo.fake_repos' },
  { filter: 'stars > 100 && owner == "anthropic"' },
  { map: ['name'] },
])

console.log('Cost:', ch.getCost())
```

### Go (server)

```go
import quark "github.com/FasadSalatov/quark/clients/go"

srv := quark.NewServer(&quark.ServerOptions{
    Secret:     []byte(os.Getenv("QUARK_SECRET")),
    SessionTTL: time.Hour,
})

srv.RegisterTool(quark.Tool{
    Name:       "echo.upper",
    Capability: "echo:invoke",
    Handler: func(ctx context.Context, in map[string]any) (any, *quark.Cost, error) {
        return strings.ToUpper(in["text"].(string)), &quark.Cost{ComputeMs: 1}, nil
    },
})

quark.RegisterDemoTools(srv)

http.Handle("/quark/ws", srv)
http.ListenAndServe(":3011", nil)
```

### Python

```bash
pip install quark-client
```

```python
import asyncio, time
from quark_client import Quark, QCT

async def main():
    token = QCT.create("secret", {
        "iss": "https://my-app.com",
        "sub": "user@example.com",
        "exp": int(time.time()) + 3600,
        "scope": ["echo:invoke"],
    })

    async with await Quark.connect(
        "wss://unyly.org/quark/ws",
        agent={"id": "my-bot", "kind": "llm", "name": "My Bot"},
        auth={"type": "bearer", "token": token},
    ) as ch:
        result = await ch.invoke("echo.upper", {"text": "hello"})
        print(result)

asyncio.run(main())
```

## Live demo

Open [unyly.org/quark](https://unyly.org/quark) → press **«connect»** → see live v1.0 frames against the reference server. Try LST, INV (with input validation), streaming, pipeline composition, subscriptions, error handling.

## Spec status

**v1.0 — Stable (2026-06-06).** Backward-compatibility guaranteed through v1.x. Breaking changes deferred to v2.0 with 12-month deprecation window.

## Test coverage

| Implementation | Tests | Status |
|---|---|---|
| Go server | 35 | ✓ all green |
| TypeScript client | 21 | ✓ all green |
| Python client | 22 | ✓ all green |
| **Total** | **78** | ✓ |

## Roadmap

- ~~v0.1 (Apr 2026) — initial draft~~
- ~~v0.2 (Jun 5, 2026) — production-grade~~
- ✅ **v1.0 (Jun 6, 2026) — stable**
- v1.1 (Q3 2026) — QUIC transport, mesh routing
- v1.2 (Q4 2026) — WebRTC P2P for browser-to-browser agents
- v1.3 (Q1 2027) — WASM pipeline stages (sandboxed user code)
- v2.0 (Q3 2027) — Asymmetric QCT signing, full CEL adoption

## Contributing

- Open issues for spec ambiguities or impl bugs
- PRs for additional client SDKs (Rust, Swift, Java) welcome
- Discussion: [unyly.org/quark](https://unyly.org/quark) or [@Fasad_Salatov](https://t.me/Fasad_Salatov)

## License

MIT — see [LICENSE](./LICENSE).

## Built by

[Unyly](https://unyly.org) — MCP marketplace by [Fasad Salatov](https://github.com/FasadSalatov).
