# @unyly/quark-client

TypeScript SDK for the **Quark Protocol** — successor to MCP.

- Streaming-first
- Pipeline composition (server-side)
- Subscriptions
- Backpressure
- Capability-based security
- WebSocket transport
- MCP-compatible adapter

**Status:** v0.1 reference implementation. Not yet stable.

## Install

```bash
pnpm add @unyly/quark-client
```

## Quick start

```typescript
import { Quark } from '@unyly/quark-client'

// Connect
const ch = await Quark.connect('wss://your-server/quark', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
  capabilities: ['github:read:*', 'slack:notify:#dev'],
})

// List tools
const tools = await ch.listTools()
console.log(tools)

// One-shot invoke
const repos = await ch.invoke('github.list_repos', { owner: 'anthropic' })

// Streaming
for await (const log of ch.stream('logs.tail', { file: 'app.log' })) {
  console.log(log.line)
}

// Server-side pipeline (the killer feature)
const filtered = await ch.pipeline([
  { tool: 'github.list_repos', input: { owner: 'anthropic' } },
  { filter: 'stars > 100' },
  { map: ['name'] },
])
// → ['claude-code', 'mcp', ...]

// Subscriptions
const sub = ch.subscribe('github.pr_opened', { repo: 'foo/bar' })
for await (const event of sub.events) {
  console.log('PR opened:', event.title)
}

// Cleanup
await ch.close()
```

## Spec

Full protocol spec: <https://unyly.org/quark> or [`docs/quark/spec.md`](../../docs/quark/spec.md).

## License

MIT — Fasad Salatov (Unyly).
