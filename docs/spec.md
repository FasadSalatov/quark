# Quark Protocol — Specification v1.0

> Built by Unyly. (c) 2026 Fasad Salatov. Streaming-first AI tool protocol with production-grade security, reliability, and composition. Stable.

**Status:** Stable v1.0 — 2026-06-06
**License:** CC BY-NC-ND 4.0 (share with attribution; no commercial use, no derivatives). Reference code in this repo: BUSL-1.1 — production/commercial use requires a commercial license.
**Repository:** <https://github.com/FasadSalatov/quark>
**Reference implementations:** Go server + TypeScript client + Python client (in this repo)
**Live demo:** <https://unyly.org/quark>

---

## Stability guarantee

v1.0 is **stable**. Breaking changes will be deferred to v2.0 with a minimum 12-month deprecation window. Non-breaking additions (new optional fields, new frame kinds with feature negotiation) may ship in v1.x minor versions.

The following are **stable**:
- Frame format (header + JSON payload, WebSocket text)
- All 14 message kinds (HEY, RSM, LST, INV, SUB, UNS, CAN, ACK, HBT, BYE, RES, STR, END, ERR, EVT, WIN, HBA)
- QCT token format
- Pipeline composition syntax
- Capability string grammar
- Error code list (15 codes)

The following are **versioned** within v1.x:
- Tool schema extensions (effects, cost, requires_capability)
- Filter expression grammar (extending toward CEL parity)
- Federation routing rules
- MessagePack binary encoding

---

## Changelog

### v1.0 (2026-06-06) — stable
- **Federation** — server-to-server routing via mesh discovery (§17)
- **MessagePack binary frames** — opt-in via `Content-Type: application/x-quark-msgpack` (§4.2)
- **Extended filter language** — string ops (`matches`, `in`, `notIn`), boolean coercion, arithmetic (§13.4)
- **Schema registry** — `$ref` to `https://schemas.quark.dev/` resolved at handshake (§12.2)
- **Stability guarantee** — first stable release
- **Compatibility test suite** — Go ↔ TS ↔ Python conformance (§19)
- **IETF draft alignment** — frame format and error semantics matched to draft `quark-protocol-00`

### v0.2 (2026-06-05) — production-grade
- Cryptographically signed capability tokens (QCT, HMAC-SHA256)
- Bearer authentication, session resume, heartbeat
- Input validation, cost tracking, distributed tracing
- Tool versioning

### v0.1 (2026-04-15) — initial draft
- Streaming, composition, subscriptions, backpressure, capabilities

---

## 1. Motivation

MCP shipped in 2024 as the default AI tool protocol. It is JSON-RPC over stdio/SSE, designed for desktop apps. Production exposes structural cracks:

1. **No native streaming** — SSE workarounds
2. **Stateless per call** — context burnt every call
3. **No composition** — N round-trips for sequential workflows
4. **No subscriptions** — polling for reactive flows
5. **No backpressure** — flood → DoS
6. **No multi-agent** — bespoke bridges
7. **No capability model** — tools can do anything
8. **No reconnection** — channel drop → state lost
9. **No cost transparency** — agents spend silently
10. **No audit trail** — compliance blocked
11. **No federation** — tool servers are islands
12. **Text only** — large binary payloads inefficient

These are architectural, not bugs. Fixing requires a new protocol. **Quark** is it.

## 2. Design goals

| Goal | Means |
|---|---|
| **Streaming-native** | Bidirectional streams first-class |
| **Composable** | Server-side pipeline syntax + federation |
| **Stateful** | Sessions resume after disconnect |
| **Subscriptions** | Push events, no polling |
| **Backpressure** | Window-based flow control |
| **Typed** | JSON Schema, federated schema registry |
| **Multi-agent** | Agent IDs, agent-to-agent calls |
| **Secure** | Signed capability tokens (QCT) |
| **Observable** | Tracing + cost in every frame |
| **Reliable** | Heartbeat + resume + replay buffer |
| **Federated** | Server-to-server mesh routing |
| **Efficient** | Optional MessagePack binary encoding |
| **MCP-compatible** | Adapter wraps MCP, zero migration cost |
| **Stable** | v1.0 guarantee — no breaking changes through v1.x |

## 3. Transport

WebSocket. `ws://server/quark/ws` or `wss://server/quark/ws` (TLS recommended).

Plain TCP allowed for server-to-server federation. QUIC reserved for v1.1+.

## 4. Frame format

### 4.1 JSON (default)

WebSocket text frames containing UTF-8 JSON. Top-level fields:

```json
{
  "v": 1,
  "kind": "INV",
  "seq": 5,
  "trace_id": "abc-123",
  "span_id": "def-456",
  "parent_span_id": "ghi-789"
}
```

Mandatory: `v` (now `1` for v1.0), `kind`, `seq`.
Optional (tracing): `trace_id`, `span_id`, `parent_span_id`.

**Note:** the `v` field tracks **protocol major version** and stays `1` throughout v1.x. Implementations MUST accept `v: 2` from v0.2 clients (last pre-stable) and treat them identically.

### 4.2 MessagePack binary

Negotiated via `Content-Type: application/x-quark-msgpack` Sec-WebSocket-Protocol subprotocol.

When negotiated, all frames are sent as WebSocket binary messages containing MessagePack-encoded objects with identical schema. Particularly valuable for:
- Large file streaming (image generation, audio chunks)
- High-throughput log tails
- Embedded clients (mobile, IoT)

Fallback to JSON if subprotocol not negotiated.

## 5. Channels

Persistent stateful connection. State preserved:
- Capability grants (until token expires)
- Subscriptions (until UNS or session expired)
- Open streams (until END / CAN / resume)
- Cost accumulator
- Federation routing table (which downstream servers this channel may invoke)

Multiplex via `seq`.

## 6. Authentication & Capability Tokens

### 6.1 Bearer auth

```json
{
  "kind": "HEY",
  "v": 1,
  "auth": { "type": "bearer", "token": "qct.v1.eyJ..." },
  "agent": { "id": "claude-desktop", "kind": "llm", "name": "Claude" }
}
```

Anonymous allowed but limited to `effects: pure|read`, no subscriptions, no resume, no federation.

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
  "max_cost_usd": 5.00,
  "federation_allowed": ["github-tools.example.com", "slack-tools.example.com"]
}
```

`federation_allowed` is new in v1.0 — restricts which federated servers this token can transit.

### 6.3 Capability strings

```
github:read:*
github:write:repo:owner/name
slack:notify:#general
*:read
```

Wildcards: `a:b:c` grants exact + descendants if `a:b:c:*`.

### 6.4 Server verification

1. Parse QCT
2. Verify HMAC signature
3. Check `iat <= now`, `nbf <= now`, `exp > now`
4. If `client_id` set, match `agent.id`
5. Store `scope` as channel capabilities
6. Store `federation_allowed` as routing whitelist

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
  "v": 1,
  "agent": { "id": "claude", "kind": "llm", "name": "Claude" },
  "auth": { "type": "bearer", "token": "qct.v1.eyJ..." },
  "supports": ["streaming", "subscribe", "compose", "capabilities", "resume", "tracing", "federation", "msgpack"]
}
```

**Server:**
```json
{
  "kind": "HEY",
  "v": 1,
  "server": { "id": "github-tools", "name": "GitHub Tools", "version": "2.1.0" },
  "supports": ["streaming", "subscribe", "compose", "capabilities", "resume", "tracing", "federation"],
  "session_id": "ses_a7b3c9d1",
  "session_ttl": 3600,
  "tools": 12,
  "topics": 4,
  "federated_servers": ["slack-tools.example.com"],
  "granted_capabilities": ["github:read:*"]
}
```

`federated_servers` lists downstream Quark servers this server can route to.

## 9. Heartbeat

Every 30s:

```json
// → { "kind": "HBT", "ts": 1700000000 }
// ← { "kind": "HBA", "ts": 1700000000 }
```

Timeouts: client no `HBA` in 60s → reconnect. Server no `HBT` in 90s → close (state held for TTL).

## 10. Session resume

```json
{
  "kind": "RSM",
  "v": 1,
  "session_id": "ses_a7b3c9d1",
  "last_seq_received": 42
}
```

Server replays buffered frames with `seq > 42`. Subscriptions and capability grants survive resume. Open streams cancelled. Buffer size: 64 frames per session.

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
      "output": { "type": "array", "items": { "$ref": "https://schemas.quark.dev/types/v1/Repo" } },
      "effects": ["network", "read"],
      "cost": { "estimate": 0.0001, "currency": "USD" },
      "streaming": true,
      "requires_capability": "github:read"
    }
  ]
}
```

### 11.1 Versioning

`{ "tool": "github.list_repos@v2" }` — specific version. No `@version` = latest.

### 11.2 Schema federation (registry)

`$ref` to standard types resolved against the public registry at `https://schemas.quark.dev/`.

Conforming servers MUST cache `$ref` resolutions for the duration of a channel.

Conforming clients MAY validate locally using cached schemas.

Standard types in v1.0:
- `User` — id, login, email (optional)
- `Repo` — name, stars, owner (User)
- `Channel` — id, name, members
- `Message` — id, content, author (User), ts
- `File` — name, mime_type, size, url
- `Error` — code, message

## 12. Invoking tools

### 12.1 One-shot

Server validates input first. On failure:
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

### 12.3 Pipeline composition

```json
{
  "kind": "INV",
  "seq": 4,
  "pipeline": [
    { "tool": "github.list_repos", "input": { "owner": "anthropic" } },
    { "filter": "stars > 100 && (owner == 'anthropic' || verified == true)" },
    { "map": ["name", "stars"] },
    { "tool": "slack.notify", "input_bind": { "items": "$prev", "channel": "#dev" } }
  ]
}
```

Stages: `tool`, `filter`, `map`, `reduce`, `input_bind` with `$prev`.

## 13. Filter expression language (v1.0)

### 13.1 Grammar

```
expression := orExpr
orExpr     := andExpr ('||' andExpr)*
andExpr    := notExpr ('&&' notExpr)*
notExpr    := '!' comparison | comparison
comparison := field op value | '(' expression ')'
op         := '>' | '<' | '>=' | '<=' | '==' | '!=' | 'contains' | 'startsWith' | 'endsWith' | 'matches' | 'in' | 'notIn'
value      := number | string | boolean | null | array
```

### 13.2 Field access

```
field            # direct
field.subfield   # nested
field[0]         # array index
```

### 13.3 Examples

```
stars > 100
stars > 100 && stars < 10000
(owner == "anthropic") || (verified == true)
name contains "claude"
description startsWith "AI"
verified == true
language in ["go", "rust", "ts"]
status notIn ["archived", "deleted"]
email matches ".*@example\\.com$"
!archived
nested.field == "value"
```

### 13.4 Arithmetic (v1.0 addition)

Inside comparison values:

```
stars > followers * 0.1
created_at > now() - 86400
```

Supported: `+ - * /`, function `now()` returns Unix seconds.

## 14. Subscriptions

```json
{
  "kind": "SUB",
  "seq": 5,
  "topic": "github.pr_opened",
  "filter": { "repo": "anthropic/claude-code" }
}
```

`EVT` until `UNS` with same seq, or channel close (survives resume).

## 15. Backpressure

```json
{ "kind": "WIN", "window": 3, "retry_after_ms": 1000 }
```

Default window: 64.

## 16. Errors

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

Stable error code list (v1.0):
- `AUTH_INVALID` — bearer token verification failed
- `AUTH_EXPIRED` — token past `exp`
- `MISSING_CAPABILITY` — capability not in granted scope
- `TOOL_NOT_FOUND` — tool name or version doesn't exist
- `INVALID_INPUT` — input doesn't match schema (includes `schema_path`)
- `TIMEOUT` — tool execution exceeded budget
- `INTERNAL` — server-side error
- `BACKPRESSURE` — too many in-flight requests
- `RATE_LIMITED` — upstream rate limit (includes `retry_after_ms`)
- `CANCELLED` — client cancelled
- `SESSION_EXPIRED` — RSM session not found
- `COST_LIMIT` — token's `max_cost_usd` exceeded
- `UNSUPPORTED_VERSION` — incompatible protocol versions
- `FEDERATION_DENIED` — server not in token's `federation_allowed`
- `FEDERATION_UNREACHABLE` — downstream federated server down

## 17. Federation (v1.0)

A Quark server may **federate** invocations to other Quark servers. This enables:
- Specialized servers (one for GitHub, one for Slack, one for AI)
- Geographic locality (route to nearest server)
- Resource isolation (heavy ML on GPU server)

### 17.1 Discovery

A server announces federated downstreams in `HEY` response:
```json
"federated_servers": ["github-tools.example.com", "slack-tools.example.com"]
```

The client can route tools to specific downstreams using the `via` field:
```json
{
  "kind": "INV",
  "seq": 5,
  "tool": "github.list_repos",
  "via": "github-tools.example.com",
  "input": { "owner": "anthropic" }
}
```

If `via` is omitted, the receiving server decides routing (typically local first, then federation).

### 17.2 Routing rules

1. Server checks if the tool is local. If yes, invoke locally.
2. If not local, check `federated_servers` for a match.
3. Verify token's `federation_allowed` includes the target.
4. Open a server-to-server Quark connection (TCP allowed), forward `INV`.
5. Pipe `RES`/`STR`/`END` back to the original client, adding `via` field.

### 17.3 Token chaining

The client's QCT is forwarded to downstreams. Downstream verifies signature using its own copy of the issuer's public key (or shared secret pre-configured). If invalid → `FEDERATION_DENIED`.

This allows **trust-on-first-use** federation: the client trusts the originating server, which transitively trusts the federation network.

## 18. Tracing

W3C Trace Context format (`trace_id`: 32 hex, `span_id`: 16 hex). Server propagates `trace_id` through pipeline stages and federation. OpenTelemetry-compatible.

## 19. Compatibility & conformance

v1.0 ships a **conformance test suite** that any implementation must pass:

```
github.com/FasadSalatov/quark/tests/conformance/
├── 01-handshake/
├── 02-qct/
├── 03-invocation/
├── 04-streaming/
├── 05-pipeline/
├── 06-subscription/
├── 07-resume/
├── 08-heartbeat/
├── 09-validation/
├── 10-federation/
└── 11-tracing/
```

Test runner harnesses:
- **Go** — `go test ./tests/conformance/...`
- **TypeScript** — `pnpm test:conformance`
- **Python** — `pytest tests/conformance/`

Cross-implementation: Go server ↔ TS client ↔ Python client must all pass.

## 20. MCP compatibility

```
[AI agent] ──Quark──> [Quark adapter] ──MCP──> [legacy MCP server]
```

Adapter converts `INV` → `tools/call`. Loses streaming/composition/subscriptions. Default permissive capabilities. Zero migration cost.

## 21. Reference implementations

| Lang | Path |
|---|---|
| Go | [`clients/go/`](../clients/go) |
| TypeScript | [`clients/typescript/`](../clients/typescript) |
| Python | [`clients/python/`](../clients/python) |

Each ships with:
- Complete protocol support
- Unit + conformance tests
- Quick start example

## 22. Roadmap post-v1.0

- **v1.1** (Q3 2026) — QUIC transport, mesh routing improvements
- **v1.2** (Q4 2026) — WebRTC P2P for browser-to-browser AI agents
- **v1.3** (Q1 2027) — WASM pipeline stages (sandboxed user code)
- **v2.0** (Q3 2027) — Asymmetric QCT signing (RSA/ECDSA), full CEL adoption, capability delegation chains

Breaking changes deferred to v2.0. v1.x will receive backward-compatible additions only.

## 23. Reference

For implementers:
- **Frame layout** — §4
- **Auth flow** — §6, §8
- **Tool registration** — §11
- **Invocation patterns** — §12
- **Federation** — §17
- **Conformance tests** — §19

## 24. Open issues

Tracked at <https://github.com/FasadSalatov/quark/issues>:
- WebRTC P2P negotiation flow (v1.2)
- WASM execution model (v1.3)
- Backward-compatible MessagePack schema evolution
- Multi-tenant federation key rotation

---

**Maintainer:** Fasad Salatov (Unyly)
**Discussion:** <https://unyly.org/quark>
**Issues:** <https://github.com/FasadSalatov/quark/issues>
**Stability guarantee:** v1.0 is stable. Breaking changes deferred to v2.0 with 12-month deprecation window.
