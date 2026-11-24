---
paths:
  - "internal/node/**"
  - "internal/syncer/**"
  - "internal/aggregation/**"
  - "internal/core/lifecycle/**"
  - "internal/core/pending/**"
  - "internal/core/clock/**"
  - "internal/core/p2p/**"
---

# Concurrency

- **Single writer.** The node's dispatch loop owns the consensus store and fork choice. Other
  goroutines (gossip validators, req/resp handlers, the syncer, workers) reach that state through
  channels and hooks, never directly.
- **The loop keeps the slot clock.** Every event it handles delays the next tick, and the loop logs
  any that take a whole interval. Expensive work, such as signature and proof verification, runs on
  a worker. cgo pins an OS thread per call: bound concurrent verifies with a fixed pool.
- **Every goroutine is tracked.** Start it on the node's `lifecycle.Group`, stop it through its
  context, and let the group's `Close` join it before the store, keys or host it uses are closed.
  `Close` doesn't cancel; cancel first.
- **Channels.** Each has one owner that closes it. A send that may block on a full channel either
  selects on the context or drops on purpose, with a metric counting the drops.
- **Locks** are never held across I/O, cgo calls or channel sends.
- **Time.** Slot, interval and duty-deadline decisions take the node's `clock.Clock`; see the
  `clock` package doc.
