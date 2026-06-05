# Twitter/X launch thread — v1.0

**Tweet 1/13** 🧵

Today I'm shipping **Quark v1.0 — stable**.

It's an open protocol replacing MCP for AI agents talking to tools.

Streaming-native. Server-side composition. Signed auth. Federation. 3 SDKs (Go/TS/Python). 78 tests. MIT.

Demo: unyly.org/quark

Why it's needed 👇

---

**Tweet 2/13**

MCP from Anthropic is THE default for AI agent tools.

But it's JSON-RPC over stdio/SSE — designed for desktop apps. In production it cracks:

❌ No native streaming
❌ Stateless per call → wasted tokens
❌ No composition → N round-trips per workflow
❌ No subscriptions
❌ No backpressure → DoS possible
❌ No multi-agent
❌ No reconnect
❌ No cost transparency

---

**Tweet 3/13**

Quark's killer feature: server-side pipeline composition.

```json
{ "kind": "INV", "pipeline": [
  { "tool": "github.list_repos" },
  { "filter": "stars > 100 && owner == 'anthropic'" },
  { "map": ["name"] },
  { "tool": "slack.notify" }
]}
```

One request. Server executes the whole chain. 4 round-trips → 1.

**~10× latency reduction in real workloads.**

---

**Tweet 4/13**

Streaming is first-class:

```
→ INV logs.tail
← STR { "line": "GET /api 200" }
← STR { "line": "GET /api 500" }
← END { "cost": { "usd": 0.001 } }
```

Every tool can stream. Cancel mid-flight. Backpressure built in.

Cost reported automatically. AI agents see real $$ in real time.

---

**Tweet 5/13**

Signed capability tokens (QCT) — v1.0 stable format:

```
qct.v1.<base64(payload)>.<base64(hmac-sha256)>
```

Payload: scope (github:read:*), expiry, max_cost_usd, federation_allowed. Server verifies. Agents can't forge.

Foundation for enterprise/compliance. SOC2 ready.

---

**Tweet 6/13**

Session resume — mobile-friendly:

iPhone in pocket → 4G handover → WebSocket drops → user opens app → client auto-RSMs with `last_seq_received` → server replays buffered frames → app continues.

Zero data loss. No reconnect logic in your code.

---

**Tweet 7/13**

NEW in v1.0: **Federation**.

Servers route invocations to other Quark servers. Specialized GitHub server + Slack server + ML-on-GPU server = a mesh.

Token chain validates trust transitively. One client connection, many backend services.

---

**Tweet 8/13**

NEW in v1.0: **MessagePack binary frames**.

Opt-in via WebSocket subprotocol. For large file streams (image generation, audio chunks, log tails) and embedded clients (mobile, IoT).

Fallback to JSON if not negotiated. Same protocol, two encodings.

---

**Tweet 9/13**

NEW in v1.0: **Extended filter language**.

```
stars > 100 && owner in ['anthropic', 'openai']
email matches '.*@example\\.com$'
meta.score > followers * 0.1
status notIn ['archived', 'deleted']
!archived && verified
```

Regex, arrays, nested fields, arithmetic, `now()`. Full grammar in spec.

---

**Tweet 10/13**

Three reference SDKs at v1.0:

🦫 Go server SDK — `go get github.com/FasadSalatov/quark/clients/go`
📦 TypeScript SDK — `pnpm add @fasad_salatov/quark-client`
🐍 Python SDK — `pip install quark-client`

All three produce identical output. Verified by cross-language conformance suite.

---

**Tweet 11/13**

Tests:
- Go: 35 unit ✓
- TypeScript: 21 unit ✓
- Python: 22 unit ✓
- Cross-language conformance: 17 × 3 ✓

**78 tests passing across 3 implementations. Spec-driven.**

If implementations diverge → conformance fails → CI blocks.

---

**Tweet 12/13**

**Stability guarantee:**

v1.0 is stable. No breaking changes through v1.x. v2.0 (Q3 2027) = 12-month deprecation window.

Build on Quark today, your code works tomorrow.

---

**Tweet 13/13**

I built this while making @unyly_org (MCP marketplace, 15k+ servers).

Hit MCP's production limits. Decided to fix them properly.

Try it:
🌐 unyly.org/quark
📦 github.com/FasadSalatov/quark
🐦 reply with your use case

—Fasad
