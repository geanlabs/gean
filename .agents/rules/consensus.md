---
paths:
  - "internal/types/**"
  - "internal/state/**"
  - "internal/transition/**"
  - "internal/forkchoice/**"
  - "internal/importer/**"
  - "internal/da/**"
  - "internal/aggregation/**"
  - "internal/store/**"
  - "internal/core/protoarray/**"
---

# Consensus

- **Read the spec first.** Before changing a protocol function, read the consensus-specs function
  it implements at the pinned commit, and the earlier forks it builds on: each fork's spec only
  states what it changes. A function is named after the spec's, with a link to it above.
- **Deterministic results.** Map iteration order, wall-clock time and goroutine timing never decide
  what a state transition or fork choice returns. Sort before iterating when order matters.
- **Edge cases:** slot 0 and genesis, empty attestation lists, the finalized boundary, duplicate and
  out-of-order messages, unknown parent or target roots. Each is accepted or rejected exactly as
  the spec does.
- **Layering.** `state`, `transition` and `forkchoice` import from gean only core, `types`,
  `crypto` and each other; `make lint` enforces this. `core/protoarray` knows nothing of
  a chain's rules, and forks are subpackages, as `AGENTS.md`'s layout describes.
