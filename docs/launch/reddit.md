# r/programming post draft (v1.0)

**Title:** Sael Protocol v1.0 — stable streaming-first replacement for MCP (Anthropic's tool protocol)

**Flair:** Open Source

---

After 3 months of work I'm publishing **Sael v1.0 stable** — an open protocol that replaces MCP for connecting AI agents to tools.

**Why bother**

MCP became the de-facto standard in 2024 for AI agent tooling. Technically it's JSON-RPC over stdio/SSE, designed for desktop apps. In production it breaks down: no native streaming, stateless per call (wastes tokens), no composition (N round-trips for simple workflows), no subscriptions, no backpressure, no multi-agent support, no reconnect, no cost transparency, no audit trail, no federation.

These aren't bugs — they're architectural consequences of JSON-RPC. Fixing requires rewriting the protocol.

**What Sael does**

1. **WebSocket transport with frames.** Multiplexed by `seq`. Bidirectional streams first-class.

2. **Server-side pipeline composition:**

```json
{
  "kind": "INV",
  "pipeline": [
    { "tool": "github.list_repos", "input": { "owner": "anthropic" } },
    { "filter": "stars > 100 && owner == 'anthropic'" },
    { "map": ["name"] },
    { "tool": "slack.notify", "input_bind": { "items": "$prev" } }
  ]
}
```

One round-trip. Server executes the whole chain. ~10× latency reduction in real workloads.

3. **Signed capability tokens (SCT, HMAC-SHA256).** Server verifies. Agents can't forge capabilities.

4. **Session resume after disconnect.** Server buffers last 64 outgoing frames. Client reconnects with `RSM { session_id, last_seq_received }` → replay → resume.

5. **Heartbeat** (HBT/HBA) — detect dead connections.

6. **JSON Schema validation** of tool input server-side, before handler.

7. **Cost tracking** in every response: `cost: { compute_ms, api_calls, usd, tokens }`.

8. **Distributed tracing** — W3C `trace_id`/`span_id`. OpenTelemetry-compatible.

9. **Federation** (v1.0) — server-to-server routing via mesh discovery.

10. **MessagePack binary frames** (v1.0) — opt-in via subprotocol.

11. **Extended filter language** (v1.0) — `matches`, `in`, `notIn`, parens, `!`, arithmetic, nested fields.

12. **MCP-compatible adapter.** Wraps existing MCP servers. Zero migration cost.

13. **Stability guarantee.** v1.0 stable. No breaking changes through v1.x.

**Open source — three SDKs**

- **Spec v1.0** (~600 lines, MIT): github.com/FasadSalatov/sael/blob/main/docs/spec.md
- **Go server SDK** (~1250 lines)
- **TypeScript client SDK** (`@fasad_salatov/sael-client@1.0.0` on npm)
- **Python client SDK** (`sael-client==1.0.0` on PyPI)
- **78 unit tests + 17 cross-language conformance tests** — all green
- **Live demo**: unyly.org/sael

**What I'd love feedback on**

- Filter language — extended grammar enough or jump to full CEL in v1.1?
- SCT vs JWT — spec-defined was right call?
- What's missing for your production use case?

Repo: https://github.com/FasadSalatov/sael
Spec: https://github.com/FasadSalatov/sael/blob/main/docs/spec.md
Live demo: https://unyly.org/sael

—Fasad (CTO Solafon, building Unyly — MCP marketplace with 15k+ servers)
