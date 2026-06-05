# Quark Protocol — Specification v0.1

> Built by Unyly. Successor to MCP. Streaming-first AI tool protocol.

**Status:** Draft v0.1 — 2026-06-05
**License:** MIT (open spec)
**Repository:** https://github.com/FasadSalatov/quark/tree/main/docs
**Reference implementations:** Go server + TypeScript client (in this repo)

---

## 1. Motivation

Model Context Protocol (MCP) by Anthropic shipped in late 2024 and became the de-facto standard for connecting AI agents to tools. It works — but it's architecturally a JSON-RPC layered on top of stdio/SSE, designed for a desktop-app world. As AI agents move into production at scale, MCP shows fundamental cracks:

1. **No native streaming.** Long-running tool calls (LLM streams, file uploads, log tails) are workarounds via SSE.
2. **Stateless per call.** Every tool invocation rebuilds context. Tokens wasted. Latency multiplied.
3. **No composition.** "Get repos > filter by stars > summarize > post to Slack" requires N round-trips, each with full payload.
4. **No subscriptions.** Reactive workflows (notify on new PR) need external poll loops.
5. **No backpressure.** A flood of requests can DoS a tool server with no graceful degradation.
6. **No native multi-agent.** Agent-to-agent communication (Claude delegates to Gemini) requires bespoke bridges.
7. **No capability model.** Tools can do anything; clients can't restrict.

These aren't bugs — they're consequences of choosing JSON-RPC as the substrate. Fixing them requires a new protocol.

**Quark** is that protocol.

## 2. Design goals

| Goal | What this means |
|---|---|
| **Streaming-native** | Bidirectional streams are first-class. Every call can stream. |
| **Composable** | Pipe syntax (`tool1 \| filter \| tool2`) evaluated server-side. |
| **Stateful sessions** | Open a channel, keep context, save tokens and latency. |
| **Subscriptions** | Subscribe to events. Server pushes. No polling. |
| **Backpressure** | Built-in flow control. Servers throttle gracefully. |
| **Typed** | JSON Schema everywhere. Composition is type-checked. |
| **Multi-agent** | Agent IDs in the protocol. Direct agent-to-agent calls. |
| **Capability-based** | Calls require capability tokens. Agents can't do what they're not allowed. |
| **MCP-compatible** | Adapter layer wraps existing MCP servers. Zero migration cost. |

## 3. Transport

**Quark uses WebSocket as the primary transport.** WebSocket gives bidirectional streaming, runs in every browser and every modern server, and survives mobile network handovers.

```
ws://server:port/quark
wss://server:port/quark   (TLS, recommended)
```

For server-to-server federation, plain TCP with the same framing is allowed for lower overhead.

Optional QUIC support is reserved for future versions (v0.3+).

## 4. Frame format

Every message on the wire is a **frame**: 4-byte header + JSON payload.

```
┌─────────┬─────────┬─────────────────────────────┐
│ version │  kind   │       payload (JSON)        │
│ 1 byte  │ 3 bytes │       variable length       │
└─────────┴─────────┴─────────────────────────────┘
```

- `version`: protocol version, currently `0x01`
- `kind`: 3-letter ASCII opcode (see §6)
- `payload`: UTF-8 JSON, MAY be empty

WebSocket text frames are used. Binary frames are reserved for future binary payload (MessagePack option in v0.2).

## 5. Channels

A **channel** is a persistent stateful connection between a client (AI agent) and a Quark server. Opened on connect, closed on disconnect.

Within a channel, state is preserved:
- Capability grants (once granted, valid for channel lifetime or until revoked)
- Subscriptions (active until explicitly unsubscribed or channel closed)
- Open tool streams (alive until completed or cancelled)

A single channel can carry multiple simultaneous calls, streams, and subscriptions, distinguished by `seq` (sequence ID).

## 6. Message kinds

Each frame's `kind` field is a 3-letter ASCII opcode:

### Client → Server

| Kind  | Meaning |
|---|---|
| `HEY` | Handshake (capabilities, version, agent identity) |
| `LST` | List available tools |
| `INV` | Invoke a tool (one-shot or stream) |
| `SUB` | Subscribe to a stream/topic |
| `UNS` | Unsubscribe |
| `CAN` | Cancel an in-flight invocation |
| `ACK` | Backpressure ack (window update) |
| `BYE` | Graceful close |

### Server → Client

| Kind  | Meaning |
|---|---|
| `HEY` | Handshake response (server capabilities) |
| `LST` | Tool list response |
| `RES` | One-shot tool response |
| `STR` | Stream chunk (one of many) |
| `END` | Stream completed |
| `ERR` | Error (invocation, subscription, or session-level) |
| `EVT` | Subscription event |
| `WIN` | Backpressure window grant |
| `BYE` | Server-initiated close |

## 7. Handshake

Every channel starts with a `HEY` exchange.

**Client:**
```json
{
  "kind": "HEY",
  "v": 1,
  "agent": {
    "id": "claude-desktop-3.7-mac",
    "kind": "llm",
    "name": "Claude Desktop"
  },
  "supports": ["streaming", "subscribe", "compose", "capabilities"]
}
```

**Server:**
```json
{
  "kind": "HEY",
  "v": 1,
  "server": {
    "id": "github-tools-v2",
    "name": "GitHub Tools",
    "version": "2.1.0"
  },
  "supports": ["streaming", "subscribe", "compose", "capabilities"],
  "tools": 12,
  "topics": 4
}
```

If versions don't match, server replies with `ERR` and closes.

## 8. Tool registration & discovery

Tools are registered server-side at startup. Clients discover via `LST`:

**Client:**
```json
{ "kind": "LST", "seq": 1 }
```

**Server:**
```json
{
  "kind": "LST",
  "seq": 1,
  "tools": [
    {
      "name": "github.list_repos",
      "description": "List repos for a user/org",
      "input": { "type": "object", "properties": { "owner": { "type": "string" } }, "required": ["owner"] },
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

Tool schemas use JSON Schema Draft 2020-12 with two extensions:
- `effects`: array of `pure | read | write | network | money | irreversible | cost`
- `cost`: estimated cost per call (helps AI agent budget)
- `requires_capability`: capability needed to invoke

## 9. Invoking tools

### 9.1 One-shot

**Client:**
```json
{
  "kind": "INV",
  "seq": 2,
  "tool": "github.list_repos",
  "input": { "owner": "anthropic" }
}
```

**Server:**
```json
{
  "kind": "RES",
  "seq": 2,
  "output": [{ "name": "claude-code", "stars": 12000, "owner": "anthropic" }]
}
```

### 9.2 Streaming

When the tool's spec advertises `streaming: true`, results come as `STR` chunks ending with `END`.

**Client:**
```json
{ "kind": "INV", "seq": 3, "tool": "logs.tail", "input": { "file": "app.log" } }
```

**Server (chunks):**
```json
{ "kind": "STR", "seq": 3, "data": { "line": "GET /api 200" } }
{ "kind": "STR", "seq": 3, "data": { "line": "GET /api 200" } }
{ "kind": "STR", "seq": 3, "data": { "line": "GET /api 500" } }
{ "kind": "END", "seq": 3 }
```

Client can cancel mid-stream via `CAN` with the same `seq`.

### 9.3 Composition (server-side pipelines)

This is **Quark's killer feature** vs MCP.

A single `INV` can describe a pipeline. The server executes the whole pipeline, only sending the final result (or final stream) back to the client. No round-trips between steps.

**Client:**
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

Stages:
- `tool` — invoke a tool, output flows to next stage
- `filter` — CEL/SQL-like expression filtering items
- `map` — project fields
- `reduce` — aggregate
- `input_bind` — bind previous stage output into the next tool's input

The pipeline runs entirely server-side. If a stage fails, the whole pipeline returns `ERR` with the failing stage indicated. Atomic.

This collapses N HTTP round-trips into one. **~10× latency reduction** in real workloads.

## 10. Subscriptions

**Client:**
```json
{
  "kind": "SUB",
  "seq": 5,
  "topic": "github.pr_opened",
  "filter": { "repo": "anthropic/claude-code" }
}
```

Server replies `RES` with subscription id, then streams `EVT`:

```json
{ "kind": "EVT", "seq": 5, "data": { "pr": 123, "title": "Fix typo", "author": "ada" } }
```

Until client sends `UNS` with the same `seq`, or channel closes.

## 11. Backpressure

When server is overloaded, it sends `WIN` with a smaller window. Client MUST not send more than `window` outstanding requests until updated.

```json
{ "kind": "WIN", "window": 3 }
```

Default window: 64. Server can shrink any time. Client must respect.

## 12. Capabilities

Quark v0.1 ships a minimal capability model. Capabilities are strings declared by tools and granted by users.

**Tool declares:** `requires_capability: "github:write:repo:foo/bar"`

**Client (after user consent):** includes capability list in `HEY`:
```json
"capabilities": [
  "github:read:*",
  "github:write:repo:foo/bar",
  "slack:notify:#dev"
]
```

Server validates capability against the granted set on each `INV`. If missing, returns `ERR` with code `MISSING_CAPABILITY`.

v0.2 will add cryptographically signed capability grants for audit/compliance use cases.

## 13. Errors

```json
{
  "kind": "ERR",
  "seq": 4,
  "code": "MISSING_CAPABILITY",
  "message": "Tool github.write_issue requires capability github:write but it's not granted",
  "stage": 1
}
```

Standard error codes:
- `MISSING_CAPABILITY` — capability not granted
- `TOOL_NOT_FOUND` — tool name doesn't exist
- `INVALID_INPUT` — input doesn't match schema
- `TIMEOUT` — tool execution exceeded budget
- `INTERNAL` — server-side error
- `BACKPRESSURE` — too many in-flight requests
- `RATE_LIMITED` — upstream API rate limit hit
- `CANCELLED` — client cancelled

## 14. Compatibility with MCP

Quark ships with an **MCP-Quark adapter**. Any existing MCP server can be wrapped:

```
[AI agent] ──Quark──> [Quark adapter] ──MCP──> [legacy MCP server]
```

The adapter:
- Converts Quark `INV` → MCP `tools/call`
- Converts MCP responses → Quark `RES`
- Loses streaming, composition, subscriptions (MCP doesn't support them)
- Logs a warning when an advanced feature is requested

This means **zero migration cost** for adoption: clients start using Quark, existing MCPs continue to work via the adapter, authors migrate to native Quark when they want the new features.

## 15. Reference implementations

Provided in this repository:

| Language | Path | Purpose |
|---|---|---|
| Go | `/clients/go/` | Server runtime + sample tools |
| TypeScript | `/packages/quark-client/` | Client SDK for AI agents |

Both are MIT licensed. Use as starting point or copy verbatim.

Sample server (Go):
```go
srv := quark.NewServer()
srv.Tool("echo.upper", "Echo input in uppercase",
  func(ctx context.Context, in map[string]any) (any, error) {
    return strings.ToUpper(in["text"].(string)), nil
  },
)
http.Handle("/quark", srv)
```

Sample client (TypeScript):
```ts
const ch = await Quark.connect("wss://server/quark")
const repos = await ch.invoke("github.list_repos", { owner: "anthropic" })
for await (const log of ch.stream("logs.tail", { file: "app.log" })) {
  console.log(log.line)
}
```

## 16. Roadmap

- **v0.1** (Q3 2026) — this spec, reference impls, MCP adapter
- **v0.2** (Q4 2026) — signed capability grants, audit log format, binary frames (MessagePack)
- **v0.3** (Q1 2027) — QUIC transport, federation (server-to-server), mesh routing
- **v1.0** (Q2 2027) — stability guarantee, IETF draft submission

## 17. Open questions

- Should pipeline stages allow arbitrary code (sandboxed)? Possibly via WASM modules.
- How does Quark interact with OpenAI's function-calling format? Adapter both ways.
- Capability grants need a trust model. v0.1 trusts the client. v0.2 will add signing.

---

**Maintainer:** Fasad Salatov (Unyly)
**Discussion:** https://unyly.org/quark (waitlist + community)
**Updates:** This document is versioned. Breaking changes bump the version field in `HEY`.
