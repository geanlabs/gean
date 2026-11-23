---
paths:
  - "internal/lean/transition/**"
  - "internal/lean/forkchoice/**"
  - "internal/lean/attestation/**"
  - "internal/lean/attestationproof/**"
  - "internal/lean/aggregation/**"
  - "internal/lean/blockbuilder/**"
  - "internal/lean/importer/**"
  - "internal/lean/store/**"
  - "internal/core/protoarray/**"
---

# Consensus

- **Read the spec first.** Before changing a protocol function, read the leanSpec function it
  implements at the commit named beside `LEAN_SPEC_FIXTURES_SHA256` in the Makefile. For a failing
  fixture, follow the `fix-spectest` skill.
- **Deterministic results.** Map iteration order, wall-clock time and goroutine timing never decide
  what a state transition or fork choice returns. Sort before iterating when order matters.
- **Edge cases:** slot 0 and genesis, empty attestation lists, the finalized boundary, duplicate and
  out-of-order messages, unknown parent or target roots. Each is accepted or rejected exactly as
  the spec does.
- **Layering.** `transition` and `forkchoice` import only what depguard's `rules-import-only-types`
  in `.golangci.yml` allows, and `protoarray` knows nothing of a chain's rules. `make lint`
  enforces the imports.
