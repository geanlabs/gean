# beacon/internal

Beacon node services, private to `beacon/` by Go's `internal` rule. Two owners:
the chain loop (`chain/`), the only writer of chain state, and the network loop
(`network/`), which owns peers, wire traffic and sync. A bounded, prioritized
`processor/` sits between them so signature verification never delays a tick.
