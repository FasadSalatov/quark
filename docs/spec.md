# Quark Protocol — Specification v0.2

> Built by Unyly. Successor to MCP. Streaming-first AI tool protocol with production-grade security and reliability.

**Status:** Draft v0.2 — 2026-06-05
**License:** MIT (open spec)
**Repository:** <https://github.com/FasadSalatov/quark>
**Reference implementations:** Go server + TypeScript client (in this repo)
**Live demo:** <https://unyly.org/quark>

---

## Changelog

### v0.2 (2026-06-05) — production-grade
- **Cryptographically signed capability tokens** (QCT) — HMAC-SHA256, scope, expiry
- **Bearer authentication** in handshake
- **Session resume** after disconnect (RSM frame)
- **Heartbeat** (HBT/HBA) — detect dead connections
- **Tool input validation** via JSON Schema (server-side, before handler)
- **Cost tracking** in response (`cost_used` per call)
- **Distributed tracing** — `trace_id` + `span_id` in every frame
- **Tool versioning** — `tool_name@version` syntax
- **Schema federation** — `$ref` to standard types
- Protocol version field bumped to `2`

### v0.1 (2026-04-15)
- Initial draft: streaming, composition, subscriptions, backpressure, capabilities

---

## 1. Motivation

MCP shipped in 2024 and became the de-facto standard. Architecturally it's JSON-RPC over stdio/SSE for desktop apps. Production exposes cracks:

1. **No native streaming** — SSE workarounds.
2. **Stateless per call** — context rebuilt every time, tokens burnt.
3. **No composition** — N round-trips per workflow.
4. **No subscriptions** — polling for reactive flows.
5. **No backpressure** — flood = DoS.
6. **No multi-agent** — bespoke bridges.
7. **No capability model** — tools do anything.
8. **No reconnection** — channel drops, state lost.
9. **No cost transparency** — agent spends $$ silently.
10. **No audit trail** — compliance can't verify.

These are architectural, not bugs. Fixing them needs a new protocol. **Quark** is it.

## 2. Design goals

| Goal | Means |
|---|---|
| **Streaming-native** | Bidirectional streams first-class |
| **Composable** | Server-side pipeline syntax |
| **Stateful** | Sessions resume after disconnect |
| **Subscriptions** | Push events, no polling |
| **Backpressure** | Window-based flow control |
| **Typed** | JSON Schema, type-checked composition |
| **Multi-agent** | Agent IDs in protocol |
| **Secure** | Signed capability tokens |
| **Observable** | Tracing + cost in every frame |
| **Reliable** | Heartbeat + resume |
| **MCP-compatible** | Adapter wraps MCP, zero migration |

## 3. Transport

WebSocket. `ws://server/quark/ws` or `wss://server/quark/ws` (TLS recommended).

Plain TCP allowed for server-to-server federation. QUIC reserved for v0.3+.

## 4. Frame format

WebSocket text frames containing UTF-8 JSON. Top-level fields:

```json
{
  "v": 2,
  "kind": "INV",
  "seq": 5,
  "trace_id": "abc-123",
  "span_id": "def-456",
  "parent_span_id": "ghi-789"
}
```

Mandatory: `v`, `kind`, `seq`.
Optional (tracing): `trace_id`, `span_id`, `parent_span_id`.

## 5. Channels

Persistent stateful connection. State preserved:
- Capability grants (until token expires)
- Subscriptions (until UNS or session expired)
- Open streams (until END / CAN / resume)
- Cost accumulator

Multiplex via `seq`.

## 6. Authentication & Capability Tokens

### 6.1 Bearer auth

```json
{
  "kind": "HEY",
  "v": 2,
  "auth": { "type": "bearer", "token": "qct.v1.eyJ..." },
  "agent": { "id": "claude-desktop", "kind": "llm", "name": "Claude" }
}
```

Anonymous allowed but limited to `effects: pure|read`, no subscriptions, no resume.

### 6.2 QCT format

```
qct.v1.<base64url(payload)>.<base64url(signature)>
```

`signature = HMAC-SHA256(secret, "v1." + base64url(payload))`

Payload:
```json
{
  "iss": "https://issuer.example.com",
  "sub": "user@example.com",
  "iat": 1690000000,
  "nbf": 1690000000,
  "exp": 1700000000,
  "scope": ["github:read:*", "slack:notify:#dev"],
  "client_id": "claude-desktop",
  "max_cost_usd": 5.00
}
```

### 6.3 Capability strings

```
github:read:*
github:write:repo:owner/name
slack:notify:#general
*:read
```

Wildcards: capability `a:b:c` grants exact + descendants if `a:b:c:*`.

### 6.4 Server verification

1. Parse QCT (split by `.`)
2. Verify HMAC signature
3. Check `iat <= now`, `nbf <= now`, `exp > now`
4. If `client_id` set, match `agent.id`
5. Store `scope` as granted capabilities

Failure → `ERR { code: "AUTH_INVALID" }` + close.

## 7. Message kinds

### Client → Server

| Kind | Meaning |
|---|---|
| `HEY` | Handshake |
| `RSM` | Resume session |
| `LST` | List tools |
| `INV` | Invoke (one-shot / stream / pipeline) |
| `SUB` | Subscribe topic |
| `UNS` | Unsubscribe |
| `CAN` | Cancel in-flight |
| `ACK` | Window update |
| `HBT` | Heartbeat |
| `BYE` | Close |

### Server → Client

| Kind | Meaning |
|---|---|
| `HEY` | Handshake response |
| `LST` | Tool list response |
| `RES` | One-shot result (+ cost) |
| `STR` | Stream chunk |
| `END` | Stream done (+ cost) |
| `ERR` | Error |
| `EVT` | Subscription event |
| `WIN` | Window grant |
| `HBA` | Heartbeat ack |
| `BYE` | Server close |

## 8. Handshake

**Client:**
```json
{
  "kind": "HEY",
  "v": 2,
  "agent": { "id": "claude", "kind": "llm", "name": "Claude" },
  "auth": { "type": "bearer", "token": "qct.v1.eyJ..." },
  "supports": ["streaming", "subscribe", "compose", "capabilities", "resume", "tracing"]
}
```

**Server:**
```json
{
  "kind": "HEY",
  "v": 2,
  "server": { "id": "github-tools", "name": "GitHub Tools", "version": "2.1.0" },
  "supports": ["streaming", "subscribe", "compose", "capabilities", "resume", "tracing"],
  "session_id": "ses_a7b3c9d1",
  "session_ttl": 3600,
  "tools": 12,
  "topics": 4,
  "granted_capabilities": ["github:read:*"]
}
```

`session_id` lets client resume. `session_ttl` = seconds state held after disconnect.

## 9. Heartbeat

Every 30s:

```json
// → { "kind": "HBT", "ts": 1700000000 }
// ← { "kind": "HBA", "ts": 1700000000 }
```

Timeouts:
- Client no `HBA` in 60s → reconnect
- Server no `HBT` in 90s → close (state held for TTL)

## 10. Session resume

After reconnect:
```json
{
  "kind": "RSM",
  "v": 2,
  "session_id": "ses_a7b3c9d1",
  "last_seq_received": 42
}
```

Server: replay buffered frames with `seq > 42`, then resume. Or `ERR { code: "SESSION_EXPIRED" }` → client falls back to fresh `HEY`.

Subscriptions and capability grants survive resume. Open streams cancelled.

Servers MUST buffer last 64 outgoing frames per session.

## 11. Tool registration & discovery

```json
{
  "kind": "LST",
  "seq": 1,
  "tools": [
    {
      "name": "github.list_repos",
      "version": "v2",
      "description": "List repos for a user/org",
      "input": {
        "type": "object",
        "properties": { "owner": { "type": "string", "minLength": 1 } },
        "required": ["owner"]
      },
      "output": { "type": "array", "items": { "$ref": "#/types/Repo" } },
      "effects": ["network", "read"],
      "cost": { "estimate": 0.0001, "currency": "USD" },
      "streaming": true,
      "requires_capability": "github:read"
    }
  ],
  "types": {
    "Repo": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "stars": { "type": "integer" },
        "owner": { "type": "string" }
      }
    }
  }
}
```

Extensions to JSON Schema:
- `effects`: `pure | read | write | network | money | irreversible | cost`
- `cost`: estimated per-call cost
- `requires_capability`: capability string

### 11.1 Versioning

`{ "tool": "github.list_repos@v2" }` — specific version. No `@version` = latest. Keep old version 1+ release cycle on breaking changes.

### 11.2 Schema federation

`$ref` to standard types (`https://schemas.quark.dev/`). v0.2 allows but doesn't enforce resolution.

## 12. Invoking tools

### 12.1 One-shot

Server validates `input` against schema first. Fail:
```json
{ "kind": "ERR", "seq": 2, "code": "INVALID_INPUT", "schema_path": "/required/0" }
```

Success:
```json
{
  "kind": "RES",
  "seq": 2,
  "output": [{ "name": "claude-code", "stars": 12000 }],
  "cost": { "compute_ms": 234, "api_calls": 1, "usd": 0.0001 }
}
```

### 12.2 Streaming

```
→ INV { "seq": 3, "tool": "logs.tail", "input": { "file": "app.log" } }
← STR { "seq": 3, "data": { "line": "..." } }
← STR { "seq": 3, "data": { "line": "..." } }
← END { "seq": 3, "cost": { "compute_ms": 5000, "usd": 0.001 } }
```

Cost reported on END for streams.

### 12.3 Pipeline composition

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

Stages: `tool`, `filter`, `map`, `reduce` (v0.3), `input_bind` with `$prev`.

Whole pipeline server-side. Atomic — stage fail = whole pipeline ERR.

### 12.4 Filter expression language

```
expression := comparison (('&&' | '||') comparison)*
comparison := field op value
op         := '>' | '<' | '>=' | '<=' | '==' | '!=' | 'contains' | 'startsWith' | 'endsWith'
value      := number | string | boolean | null
```

```
stars > 100
stars > 100 && stars < 10000
owner == "anthropic"
name contains "claude"
verified == true
```

v0.3 will adopt full Google CEL.

## 13. Subscriptions

```json
{
  "kind": "SUB",
  "seq": 5,
  "topic": "github.pr_opened",
  "filter": { "repo": "anthropic/claude-code" }
}
```

`EVT` until `UNS` with same seq, or channel close (survives resume).

## 14. Backpressure

```json
{ "kind": "WIN", "window": 3 }
```

Default window 64. Rate-limit hint:
```json
{ "kind": "WIN", "window": 1, "retry_after_ms": 1000 }
```

## 15. Errors

```json
{
  "kind": "ERR",
  "seq": 4,
  "code": "MISSING_CAPABILITY",
  "message": "tool requires github:write but only github:read granted",
  "stage": 1,
  "trace_id": "abc-123"
}
```

Codes:
- `AUTH_INVALID`, `AUTH_EXPIRED`
- `MISSING_CAPABILITY`
- `TOOL_NOT_FOUND` (or version not found)
- `INVALID_INPUT` (with `schema_path`)
- `TIMEOUT`, `INTERNAL`
- `BACKPRESSURE`, `RATE_LIMITED`
- `CANCELLED`
- `SESSION_EXPIRED`
- `COST_LIMIT` (token's `max_cost_usd` would be exceeded)
- `UNSUPPORTED_VERSION`

## 16. Tracing

```json
{
  "kind": "INV",
  "seq": 5,
  "trace_id": "abc-123",
  "span_id": "def-456",
  "parent_span_id": "ghi-789",
  "tool": "github.list_repos"
}
```

W3C Trace Context format (32 hex / 16 hex). Server propagates `trace_id` through pipeline stages and federation. OpenTelemetry-compatible.

## 17. MCP compatibility

```
[AI agent] ──Quark──> [Quark adapter] ──MCP──> [legacy MCP server]
```

Adapter converts `INV` → `tools/call`, MCP responses → `RES`. Loses streaming/composition/subscriptions. Default permissive capabilities (MCP has none). Zero migration cost.

## 18. Reference implementations

| Lang | Path |
|---|---|
| Go | [`clients/go/`](../clients/go) |
| TypeScript | [`clients/typescript/`](../clients/typescript) |

Sample Go server:
```go
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

http.Handle("/quark/ws", srv)
```

Sample TS client:
```ts
import { Quark, QCT } from '@unyly/quark-client'

const token = QCT.create({
  secret: process.env.QUARK_SECRET!,
  payload: {
    iss: 'https://my-app.com',
    sub: 'user@example.com',
    exp: Math.floor(Date.now() / 1000) + 3600,
    scope: ['echo:invoke'],
  },
})

const ch = await Quark.connect('wss://server/quark/ws', {
  agent: { id: 'my-bot', kind: 'llm', name: 'My Bot' },
  auth: { type: 'bearer', token },
})

const upper = await ch.invoke('echo.upper', { text: 'hello' })
await ch.close()
```

## 19. Roadmap

- **v0.1** (Apr 2026) — initial draft
- **v0.2** (Jun 2026) — **this** — auth, resume, heartbeat, validation, cost, tracing
- **v0.3** (Q4 2026) — CEL, federation, MessagePack, schema registry
- **v0.4** (Q1 2027) — QUIC, WebRTC, WASM stages
- **v1.0** (Q2 2027) — stability, IETF draft, audit certification

## 20. Open questions

- Sandboxed WASM pipeline stages (v0.4)?
- OpenAI function-calling format adapter both ways?
- Asymmetric (RSA/ECDSA) signing for QCT (v0.3)?
- Cross-issuer trust for federation (TBD)?

---

**Maintainer:** Fasad Salatov (Unyly)
**Discussion:** <https://unyly.org/quark>
**Issues:** <https://github.com/FasadSalatov/quark/issues>
