// Package sael — Go reference implementation of the Sael Protocol v1.0.
//
// Spec: https://github.com/FasadSalatov/sael/blob/main/docs/spec.md
//
// v1.0 features:
//   - Cryptographically signed capability tokens (SCT, HMAC-SHA256)
//   - Bearer auth, session resume, heartbeat
//   - Tool input validation (JSON Schema)
//   - Cost tracking + W3C distributed tracing
//   - Tool versioning (name@version)
//   - **Federation** — server-to-server routing
//   - **MessagePack binary frames** (opt-in via subprotocol)
//   - **Extended filter language** — matches/in/notIn, parens, !, arithmetic
//   - **Schema registry** — $ref resolution
//   - **Stability guarantee** — v1.x backward-compatible
package sael

// ProtocolVersion is the major version of the Sael protocol this
// implementation speaks (v1.x is stable).
const ProtocolVersion = 1
