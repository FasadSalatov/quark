# @fasad_salatov/quark-client

TypeScript SDK for the **Quark Protocol v0.2** — streaming-first AI tool protocol replacing MCP.

## v0.2 features

- **Signed capability tokens** (QCT) — HMAC-SHA256, scope, expiry
- **Bearer authentication** in handshake
- **Auto-reconnect with session resume** — survives mobile network drops
- **Heartbeat** — detect dead connections
- **Server-side pipeline composition** — N round-trips → 1
- **Streaming + subscriptions** — first-class
- **Cost tracking** — running accumulator
- **Distributed tracing** — W3C trace_id / span_id

## Install

```bash
pnpm add @fasad_salatov/quark-client
```

## Quick start with auth

```typescript
import { Quark, QCT } from '@fasad_salatov/quark-client'

// 1. Mint a token (server has the secret too)
const token = await QCT.create({
  secret: process.env.QUARK_SECRET!,
  payload: {
    iss: 'https://my-app.com',
    sub: 'user@example.com',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['github:read:*', 'slack:notify:#dev'],
    max_cost_usd: 5.00,
  },
})

// 2. Connect with bearer auth
const ch = await Quark.connect('wss://server/quark/ws', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
  auth: { type: 'bearer', token },
})

// 3. Use it
const tools = await ch.listTools()

const repos = await ch.invoke('github.list_repos', { owner: 'anthropic' })

for await (const log of ch.stream('logs.tail', { file: 'app.log' })) {
  console.log(log)
}

const filtered = await ch.pipeline([
  { tool: 'github.list_repos', input: { owner: 'anthropic' } },
  { filter: 'stars > 100' },
  { map: ['name'] },
])

const sub = ch.subscribe('github.pr_opened', { repo: 'anthropic/claude-code' })
for await (const event of sub.events) console.log('PR:', event)

console.log('Total cost:', ch.getCost())  // { compute_ms, api_calls, usd, tokens }

await ch.close()
```

## Distributed tracing

```typescript
import { newTraceId, newSpanId } from '@fasad_salatov/quark-client'

const trace_id = newTraceId()

await ch.invoke('a.tool', input, { trace_id, span_id: newSpanId() })
await ch.invoke('b.tool', otherInput, { trace_id, span_id: newSpanId() })
```

All frames with the same `trace_id` are correlated by OpenTelemetry collectors.

## Tool versioning

```typescript
// Use specific version
await ch.invoke('github.list_repos@v2', { owner })

// Or use latest
await ch.invoke('github.list_repos', { owner })
```

## Auto-reconnect

By default, the client auto-reconnects with session resume after network drops. Disable:

```typescript
const ch = await Quark.connect(url, {
  agent: { ... },
  autoReconnect: false,
})
```

## Spec

Full protocol spec: [v0.2 spec](https://github.com/FasadSalatov/quark/blob/main/docs/spec.md).

## License

MIT — Fasad Salatov (Unyly).
