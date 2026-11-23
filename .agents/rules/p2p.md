---
paths:
  - "internal/core/p2p/**"
  - "internal/lean/p2p/**"
  - "internal/lean/syncer/**"
  - "internal/lean/api/**"
  - "internal/core/httpapi/**"
---

# Networking and peer input

- **Peer input is hostile.** Gossip, req/resp and API input is validated before use, and sizes are
  checked against the transport limits (the `Max*Size` constants in `core/p2p` and `lean/p2p`)
  before allocating. A malformed message is rejected; it never panics the node.
- **Whose fault.** A peer's bad message is logged with `logger.Warn` and the peer id, never with
  `logger.Error`, which is for our own faults.
- **Layering.** `core/p2p` is the chain-agnostic transport: gossipsub parameters, the message-id
  function, snappy framing and req/resp deadlines. Topics, protocols and message handling live in
  `lean/p2p`.
- **Req/resp streams** always have a deadline and are closed or reset on every path.
- **HTTP response bodies** are closed on every path.
- **Tests** connect hosts over `core/p2p/memnet`, not sockets.
