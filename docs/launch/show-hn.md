# Show HN: Quark — Streaming-first AI Tool Protocol (Successor to MCP) — v1.0 Stable

> **Title (HN):** Show HN: Quark v1.0 — streaming-first protocol for AI agents talking to tools (successor to MCP)
>
> **URL:** https://unyly.org/quark

Hi HN,

I've been building **Quark** — an open protocol that replaces MCP (Model Context Protocol from Anthropic) for connecting AI agents to tools.

**Today I'm shipping v1.0 — stable.** No breaking changes through v1.x. Three reference SDKs (Go, TypeScript, Python). 78 tests passing. Cross-language conformance suite.

## Why Quark exists

MCP shipped in late 2024 and became the default. It's JSON-RPC over stdio/SSE, designed for desktop apps. In production it cracks:

- No native streaming — SSE workarounds for long-running ops
- Stateless per call — context burnt every invocation
- No composition — `get repos → filter → notify Slack` = 4 round-trips (~800ms)
- No subscriptions — reactive workflows via external polling
- No backpressure — flood = DoS
- No multi-agent — Claude can't delegate to Gemini without bespoke bridges
- No capability model — tools can do anything
- No reconnect — channel drops, state lost
- No cost transparency — agents spend $$ silently
- No audit trail — compliance blocked
- No federation — tool servers are islands

These aren't bugs, they're consequences of JSON-RPC. So I designed Quark from scratch with WebSocket as the transport.

## What Quark does differently

**Server-side pipeline composition** — single `INV` describes the whole workflow:

```json
{
  "kind": "INV",
  "seq": 4,
  "pipeline": [
    { "tool": "github.list_repos", "input": { "owner": "anthropic" } },
    { "filter": "stars > 100 && owner == 'anthropic'" },
    { "map": ["name"] },
    { "tool": "slack.notify", "input_bind": { "items": "$prev", "channel": "#dev" } }
  ]
}
```

4 round-trips → 1. ~800ms → ~80ms in real workloads. **10× latency reduction.**

**Streaming-native** — every tool can stream:

```json
// → INV { "seq": 3, "tool": "logs.tail" }
// ← STR { "data": { "line": "GET /api 200" } }
// ← STR { "data": { "line": "GET /api 500" } }
// ← END { "cost": { "compute_ms": 5000, "usd": 0.001 } }
```

**Signed capability tokens (QCT)** — HMAC-SHA256, scope, expiry, max_cost_usd. Servers verify, agents can't forge:

```
qct.v1.eyJpc3MiOiJodHRwczovL215LWFwcC5jb20iLCJzdWIiOiJ1c2VyIiwiZXhwIjoxNzAwLCJzY29wZSI6WyJnaXRodWI6cmVhZDoqIl19.dW5pdHk
```

**Session resume** after disconnect — mobile-friendly. iPhone switches from WiFi to 4G, WebSocket drops, client auto-`RSM`s with `last_seq_received` → server replays buffered frames → no data loss.

**Cost tracking + W3C tracing** — every `RES`/`END` returns `cost: { compute_ms, api_calls, usd, tokens }`. Every frame has `trace_id`/`span_id`. OpenTelemetry-compatible.

**Federation** (v1.0) — server-to-server routing. Specialized servers (one for GitHub, one for Slack, one for ML on GPU box) connect into a mesh.

**MessagePack binary frames** (v1.0) — opt-in via subprotocol negotiation. For large file streams, audio chunks, embedded clients.

**Extended filter language** (v1.0) — `matches` regex, `in`/`notIn` arrays, parens, `!`, arithmetic, nested fields, `now()`.

**MCP-compatible adapter** — wraps existing MCP servers. Clients use Quark, legacy MCPs keep working. **Zero migration cost.**

**Stability guarantee** — v1.0 is stable. Breaking changes deferred to v2.0 with 12-month deprecation window.

## Try it

- **Live demo** in browser: https://unyly.org/quark — press "connect", you'll see real v1.0 WebSocket frames against the reference Go server with cost accumulator, heartbeat, validation demo
- **Spec v1.0** (~600 lines, MIT): https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
- **TypeScript SDK** (`@fasad_salatov/quark-client@1.0.0`): https://www.npmjs.com/package/@fasad_salatov/quark-client
- **Go server SDK**: https://github.com/FasadSalatov/quark/tree/main/clients/go
- **Python SDK** (`quark-client==1.0.0`): https://pypi.org/project/quark-client/
- **Conformance test suite**: 78 unit tests + 17 cross-language conformance tests — all three SDKs produce identical output

```bash
pnpm add @fasad_salatov/quark-client
# or: pip install quark-client
```

```ts
import { Quark, QCT } from '@fasad_salatov/quark-client'

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

const repos = await ch.invoke('demo.fake_repos')

const filtered = await ch.pipeline([
  { tool: 'demo.fake_repos' },
  { filter: 'stars > 100' },
  { map: ['name'] },
])

console.log('Cost:', ch.getCost())  // running accumulator
```

## What I'd love feedback on

1. **The filter syntax** — v1.0 ships an extended language (`matches`, `in`, `notIn`, arithmetic). Is this enough or should I jump to full CEL (Google Common Expression Language) in v1.1?
2. **QCT vs JWT** — I chose to spec-define QCT for Quark instead of reusing JWT. Was that the right call?
3. **MCP adapter** — opinions on the trade-off (zero migration cost vs no v1.0 features through the adapter)?
4. **Federation model** — v1.0 ships routing surface but no built-in connection pooling. Should v1.1 ship pooling, or leave to userland?
5. **What's missing** for your production use case?

## Roadmap

- ✓ v0.1 (Apr 2026) — initial draft
- ✓ v0.2 (Jun 5, 2026) — production-grade
- ✓ **v1.0 (Jun 6, 2026) — stable** ← this release
- v1.1 (Q3 2026) — QUIC transport, mesh routing pooling
- v1.2 (Q4 2026) — WebRTC P2P for browser-to-browser agents
- v1.3 (Q1 2027) — WASM pipeline stages (sandboxed user code)
- v2.0 (Q3 2027) — Asymmetric QCT signing, full CEL, capability delegation chains

Background: I'm Fasad ([@FasadSalatov](https://github.com/FasadSalatov)), CTO at Solafon, 11 years writing software. Quark grew out of frustration while building [Unyly](https://unyly.org) — an MCP marketplace with 15k+ servers. We hit MCP's production limits and decided to fix them properly.

Repo, spec, three SDKs, tests, demo: https://github.com/FasadSalatov/quark
Site: https://unyly.org/quark
Live demo: https://unyly.org/quark — try it in browser, no install needed

Happy to answer anything. Specifically interested in: what are you using AI agents + tools for in production today, and where does MCP frustrate you?

—Fasad
