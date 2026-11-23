---
paths:
  - "internal/lean/types/**"
---

# SSZ types

- **Generated code.** Never edit `internal/lean/types/*_encoding.go`: change the struct tags and run
  `make lean-sszgen`, whose output CI diffs. A new top-level type goes on the `sszgen` line for its
  file in that target.
- **Match the spec.** Field order, field types and `ssz-size`/`ssz-max` tags are the leanSpec
  container's, in its order. A reordered field changes every root.
- **Bottom layer.** `lean/types` imports no other gean package; `make lint` enforces this.
- **Bounded decoding.** A length read from the wire is checked against the list's maximum before
  anything is allocated.
