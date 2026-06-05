// Package quark — Go reference implementation of the Quark Protocol v1.0.
//
// Spec: https://github.com/FasadSalatov/quark/blob/main/docs/spec.md
//
// v1.0 features:
//   - Cryptographically signed capability tokens (QCT, HMAC-SHA256)
//   - Bearer auth, session resume, heartbeat
//   - Tool input validation (JSON Schema)
//   - Cost tracking + W3C distributed tracing
//   - Tool versioning (name@version)
//   - **Federation** — server-to-server routing
//   - **MessagePack binary frames** (opt-in via subprotocol)
//   - **Extended filter language** — matches/in/notIn, parens, !, arithmetic
//   - **Schema registry** — $ref resolution
//   - **Stability guarantee** — v1.x backward-compatible
package quark

// ProtocolVersion is the major version of the Quark protocol this
// implementation speaks (v1.x is stable).
const ProtocolVersion = 1
