# Twitter/X launch thread

Use as starting point — edit timing/voice.

---

**Tweet 1/12** 🧵

Spent 3 months designing a new protocol to replace MCP.

It's called Quark. Streaming-first. Server-side composition. Signed capability tokens. Open & MIT.

Demo + spec: unyly.org/quark

Why it's needed 👇

---

**Tweet 2/12**

MCP from Anthropic is THE default for AI agent tools.

But it's JSON-RPC over stdio/SSE — designed for desktop apps. In production it cracks:

- No native streaming → SSE hacks
- Stateless per call → wasted tokens
- No composition → N round-trips per workflow
- No subscriptions
- No backpressure → DoS possible
- No multi-agent
- No reconnect

---

**Tweet 3/12**

Quark's killer feature: server-side pipeline composition.

```json
{ "kind": "INV", "pipeline": [
  { "tool": "github.list_repos" },
  { "filter": "stars > 100" },
  { "map": ["name"] },
  { "tool": "slack.notify" }
]}
```

One request. Server executes the whole chain. 4 round-trips → 1.

**~10× latency reduction in real workloads.**

---

**Tweet 4/12**

Streaming is first-class:

```
→ INV logs.tail
← STR { "line": "GET /api 200" }
← STR { "line": "GET /api 500" }
← END { "cost": { "usd": 0.001 } }
```

Every tool can stream. Cancel mid-flight. Backpressure built in.

Cost is reported automatically. AI agents see real $$ in real time.

---

**Tweet 5/12**

Signed capability tokens (QCT):

```
qct.v1.<base64(payload)>.<base64(hmac-sha256)>
```

Payload includes scope (`github:read:*`), expiry, max_cost_usd. Server verifies. Agents can't forge.

Foundation for enterprise/compliance use cases. SOC2, GDPR, audit-ready.

---

**Tweet 6/12**

Session resume — mobile-friendly:

iPhone in pocket → 4G handover → WebSocket drops → user opens app → client auto-RSMs → last 30s of missed push events arrive at once.

Zero data loss. No reconnect logic in your code.

---

**Tweet 7/12**

Distributed tracing built into the protocol. Every frame can carry W3C `trace_id` / `span_id` / `parent_span_id`.

OpenTelemetry collectors ingest Quark traces via a sidecar. Full audit trail of who-what-when. Compliance teams love it.

---

**Tweet 8/12**

Multi-agent native: agents can call each other through Quark.

Claude in Cursor → delegate to Gemini in browser → results back.

No bespoke bridges. Just `agent` IDs in the protocol.

---

**Tweet 9/12**

MCP-compatible adapter ships day one.

```
[AI agent] ─Quark─> [Adapter] ─MCP─> [legacy MCP server]
```

You start using Quark. Existing MCPs keep working. Authors migrate to native Quark when they want streaming/composition/auth.

**Zero migration cost.**

---

**Tweet 10/12**

Reference implementations included, MIT:

🦫 Go server SDK (~1250 lines)
📦 TypeScript client (`@unyly/quark-client`)
✅ 32 tests, all green
📜 ~500-line spec

Live demo in browser: unyly.org/quark — press "connect" and click buttons. Real frames, real server.

---

**Tweet 11/12**

Roadmap:

- v0.1 (Apr) — initial draft
- **v0.2 (today) — production-grade**
- v0.3 (Q4 2026) — Google CEL, federation, MessagePack
- v0.4 (Q1 2027) — QUIC, WebRTC, WASM pipeline stages
- v1.0 (Q2 2027) — IETF draft submission, audit certification

---

**Tweet 12/12**

I built this while making @unyly_org (MCP marketplace, 15k+ servers).

Hit MCP's production limits. Decided to fix them properly.

Try it:
🌐 unyly.org/quark
📦 github.com/FasadSalatov/quark
🐦 reply with your use case — would love feedback

—Fasad

---

Use #BuildInPublic, tag @anthropic, @anthropic_ai for visibility if appropriate.
