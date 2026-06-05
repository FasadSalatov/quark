# r/programming post draft

**Title:** Quark Protocol — streaming-first replacement for MCP (Anthropic's tool protocol)

**Flair:** Open Source

---

After 3 months of work I've published Quark — an open protocol that replaces MCP for connecting AI agents to tools.

**Why bother**

MCP became the de-facto standard in 2024 for AI agent tooling. Technically it's JSON-RPC over stdio/SSE, designed for desktop apps. In production it breaks down: no native streaming, stateless per call (wastes tokens), no composition (N round-trips for simple workflows), no subscriptions, no backpressure, no multi-agent support, no reconnect, no cost transparency, no audit trail.

These aren't bugs — they're architectural consequences of JSON-RPC. Fixing requires rewriting the protocol.

**What Quark does**

1. **WebSocket transport with frames.** Multiplexed by `seq`. Bidirectional streams first-class.

2. **Server-side pipeline composition:**

```json
{
  "kind": "INV",
  "pipeline": [
    { "tool": "github.list_repos", "input": { "owner": "anthropic" } },
    { "filter": "stars > 100" },
    { "map": ["name"] },
    { "tool": "slack.notify", "input_bind": { "items": "$prev" } }
  ]
}
```

One round-trip. Server executes the whole chain. ~10× latency reduction in real workloads.

3. **Signed capability tokens (QCT, HMAC-SHA256).** Server verifies. Agents can't forge capabilities. Foundation for enterprise/compliance.

4. **Session resume after disconnect.** Server buffers last 64 outgoing frames. Client reconnects with `RSM { session_id, last_seq_received }` → replay → resume. Mobile-friendly.

5. **Heartbeat** (HBT/HBA) — detect dead connections.

6. **JSON Schema validation** of tool input server-side, before handler.

7. **Cost tracking** in every response: `cost: { compute_ms, api_calls, usd, tokens }`. Running accumulator on the client.

8. **Distributed tracing** — W3C `trace_id`/`span_id` in every frame. OpenTelemetry-compatible.

9. **Multi-agent** — agents can call each other through the protocol.

10. **MCP-compatible adapter.** Wraps existing MCP servers. Zero migration cost.

**Open source**

- Spec v0.2 (~500 lines, MIT): github.com/FasadSalatov/quark/blob/main/docs/spec.md
- Go server reference (~1250 lines)
- TypeScript client SDK (`@fasad_salatov/quark-client`)
- 32 tests, all passing
- Live demo: unyly.org/quark

**What I'd love feedback on**

- Composition syntax — is filter/map/reduce over JSON enough or should I jump to full CEL for v0.3?
- QCT vs JWT — I chose spec-defined tokens. Right call?
- What's missing for your production use case?

Repo: https://github.com/FasadSalatov/quark
Spec: https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
Live demo: https://unyly.org/quark

—Fasad (CTO Solafon, building Unyly — MCP marketplace with 15k+ servers)
