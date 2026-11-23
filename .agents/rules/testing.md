---
paths:
  - "**/*_test.go"
  - "testing/**"
---

# Testing

- **Standard library only.** Tests use `testing` and `t.Fatalf`/`t.Errorf`; no assertion libraries.
- **Fewer, broader tests.** Cases that differ only in inputs share one table-driven test with
  `t.Run`. Before adding a test, extend an existing one if it covers the same code.
- **Regression tests** fail without the fix.
- **No timing guesses.** Wait on a condition with a deadline (a polling helper, a channel, a fake
  clock), never a fixed sleep that assumes work has finished. Tests pass with `-count=1`
  repeatedly and under `-race`, as CI runs them.
- **Build tags:**
  - `fakecrypto`: fake XMSS, for multi-node tests (`make lean-e2e`).
  - `spectests`: leanSpec fixtures (`make lean-spectest`).
  - `sim`: in-process simulations (`make test-all`).
  - `aggregation_diagnostic`: a real-crypto aggregation check, run by hand.
- **What to run:** the `prep-commit` skill's table maps changed packages to make targets.
- **Spec fixtures.** Every fixture directory has a runner or a listed exclusion with a reason, and a
  skipped case names a listed skip reason; `TestSpec` fails otherwise.
