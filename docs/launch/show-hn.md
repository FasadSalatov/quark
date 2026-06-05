# Show HN: Quark — Streaming-first AI Tool Protocol (Successor to MCP)

> **Title (HN):** Show HN: Quark — streaming-first protocol for AI agents talking to tools (replaces MCP)
>
> **URL:** https://unyly.org/quark

Hi HN,

I've been building **Quark** — an open protocol that replaces MCP (Model Context Protocol from Anthropic) for connecting AI agents to tools.

MCP shipped in late 2024 and became the default. It's JSON-RPC over stdio/SSE, designed for desktop apps. It works fine for hobby setups but cracks in production:

- No native streaming — SSE workarounds for long-running ops
- Stateless per call — context burnt every invocation, tokens wasted
- No composition — `get repos → filter → notify Slack` = 4 round-trips (~800ms)
- No subscriptions — reactive workflows via external polling
- No backpressure — flood = DoS
- No multi-agent — Claude can't delegate to Gemini without bespoke bridges
- No capability model — tools can do anything
- No reconnect — channel drops, state lost
- No cost transparency — agents spend $$ silently
- No audit trail — compliance blocked

These aren't bugs, they're consequences of JSON-RPC as the substrate. So I designed Quark from scratch with WebSocket as the transport.

## What Quark does differently

**Server-side pipeline composition** — single `INV` describes the whole workflow:

```json
{
  "kind": "INV",
  "seq": 4,
  "pipeline": [
    { "tool": "github.list_repos", "input": { "owner": "anthropic" } },
    { "filter": "stars > 100" },
    { "map": ["name"] },
    { "tool": "slack.notify", "input_bind": { "items": "$prev", "channel": "#dev" } }
  ]
}
```

4 round-trips → 1. ~800ms → ~80ms in real workloads. **10× latency reduction.**

**Streaming-native** — every tool can stream:

```json
// → INV { "seq": 3, "tool": "logs.tail", "input": { "file": "app.log" } }
// ← STR { "seq": 3, "data": { "line": "GET /api 200" } }
// ← STR { "seq": 3, "data": { "line": "GET /api 500" } }
// ← END { "seq": 3, "cost": { "compute_ms": 5000, "usd": 0.001 } }
```

**Signed capability tokens (QCT)** — HMAC-SHA256, scope, expiry, max_cost_usd. Servers verify, agents can't forge:

```
qct.v1.eyJpc3MiOiJodHRwczovL215LWFwcC5jb20iLCJzdWIiOiJ1c2VyQHgiLCJleHAiOjE3MDAwMDAwMDAsInNjb3BlIjpbImdpdGh1YjpyZWFkOioiXX0.dW5pdHk
```

**Session resume** after disconnect — mobile-friendly:

```json
{ "kind": "RSM", "session_id": "ses_a7b3c9d1", "last_seq_received": 42 }
```

Server replays buffered frames with `seq > 42`, then resumes. Subscriptions and capability grants survive resume.

**Cost tracking + tracing** — every `RES`/`END` returns `cost: { compute_ms, api_calls, usd, tokens }`. Every frame has W3C `trace_id`/`span_id`. OpenTelemetry-compatible.

**MCP compatibility** — adapter wraps existing MCP servers. Clients use Quark, legacy servers keep working. Zero migration cost.

## Try it

- **Live demo** in browser: https://unyly.org/quark — press "connect", you'll see real WebSocket frames against the reference Go server
- **Spec v0.2** (~500 lines, MIT): https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
- **TypeScript SDK** (`@fasad_salatov/quark-client`): https://github.com/FasadSalatov/quark/tree/main/clients/typescript
- **Go server SDK**: https://github.com/FasadSalatov/quark/tree/main/clients/go
- **Reference test suite**: 22 Go tests + 10 TypeScript tests, all green

```bash
pnpm add @fasad_salatov/quark-client
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

1. **The composition syntax** — is `filter`/`map`/`reduce` over JSON enough, or should I jump to full CEL (Google Common Expression Language) for v0.3?
2. **QCT vs JWT** — I chose to spec-define QCT for Quark instead of reusing JWT. Was that the right call?
3. **MCP adapter** — opinions on the trade-off (zero migration cost vs no v0.2 features through the adapter)?
4. **What's missing** for your use case? I want to ship v1.0 (stable, IETF draft) by Q2 2027 and don't want to miss critical features.

## Roadmap

- v0.1 (Apr 2026) — initial draft
- **v0.2 (Jun 2026) — this release** — auth, resume, heartbeat, validation, cost, tracing
- v0.3 (Q4 2026) — CEL, federation, MessagePack, schema registry
- v0.4 (Q1 2027) — QUIC, WebRTC, WASM pipeline stages
- v1.0 (Q2 2027) — stability, IETF draft, audit certification

Background: I'm Fasad ([@FasadSalatov](https://github.com/FasadSalatov)), CTO at Solafon, 11 years writing software. Quark grew out of frustration while building [Unyly](https://unyly.org) — an MCP marketplace with 15k+ servers. We hit MCP's production limits and decided to fix them properly.

Repo, spec, SDKs, tests, demo: https://github.com/FasadSalatov/quark
Site: https://unyly.org/quark

Happy to answer anything. Specifically interested in: what are you using AI agents + tools for in production today, and where does MCP frustrate you?

—Fasad
