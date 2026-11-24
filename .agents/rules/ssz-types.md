---
paths:
  - "internal/types/**"
---

# SSZ types

- **Match the spec.** Field order, field types and list and vector limits are the spec container's,
  in its order. A reordered field changes every root.
- **Bottom layer.** `types` imports from gean only core, `types` and `crypto`; `make lint`
  enforces this.
- **Bounded decoding.** A length read from the wire is checked against the list's maximum before
  anything is allocated.
