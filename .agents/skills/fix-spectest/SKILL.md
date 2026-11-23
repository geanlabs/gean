---
name: fix-spectest
description: Debug and fix failing leanSpec fixture tests in testing/spectest/lean. Use when make lean-spectest fails, when a spec test fails after a leanSpec bump, or when asked to "fix the spec tests".
---

# Fix failing leanSpec fixture tests

The fixture tests run gean against leanSpec's published fixture file, pinned by
`LEAN_SPEC_FIXTURES_SHA256` in the Makefile; the commit it was built from is named beside it. A failure means gean and the spec disagree; the job is to
find out which one is wrong, and fix gean only when gean is.

## Phase 1: Run the failing test alone

Fixtures live in `leanSpec/fixtures/`, downloaded and checked by `make leanSpec/fixtures`, which
`make lean-spectest` runs first. `TestSpec` runs every fixture directory through the runner
registered for it in `spec_test.go`, one file per directory (`state_transition_test.go` runs
`state_transition`), or leaves the directory out with a reason listed in `excludedDirectories`.
Every run ends with a summary: cases passed, failed and skipped per directory, why each case skipped
(Poseidon XMSS, leanSpec#1210, or not implemented in gean), the excluded directories, and any
fixture expectations gean does not evaluate yet. The run fails when a directory has neither a runner
nor an exclusion, a runner or exclusion has no fixtures, a case skips without a listed reason, or a
fixture has a field gean does not declare.

```bash
make lean-spectest                                                  # everything, with the summary
cd testing/spectest/lean && go test -tags=spectests -count=1 -v -run 'TestSpec/state_transition/<case>'
```

| Fixture directory, under `leanSpec/fixtures/consensus/` | gean code |
|---|---|
| `state_transition`, `justifiability` | `internal/lean/transition` |
| `fork_choice` | `internal/lean/forkchoice`, `internal/core/protoarray`, `internal/lean/store` |
| `ssz` | `internal/lean/types` (generated `*_encoding.go`) |
| `verify_signatures`, `verify_single_message_proofs` | `internal/lean/attestation`, `internal/core/crypto/xmss` |
| `networking_codec` | `internal/core/p2p`, `internal/lean/p2p` |
| `sync` | the spec's structural checkpoint check, `verifyCheckpointStructure` in `sync_test.go`, which is narrower than `internal/lean/syncer/checkpoint` |
| `api_endpoint` | `internal/lean/api` |
| `slot_clock` | `internal/lean/types` (slot and interval math) |

## Phase 2: Classify the failure

- **Fixture fails to load or decode:** the fixture format changed with the spec; an unknown field
  names exactly what. Fix the harness in `testing/spectest/lean/` (or `internal/lean/specfixtures`),
  not consensus code. Declare a new expectation gean cannot check yet as `specfixtures.Unchecked`,
  so the summary keeps reporting it.
- **State root or post-state mismatch:** the state transition diverges. Compare field by field to
  find the first differing field; it names the rule.
- **Block accepted that should be rejected, or the reverse:** a validity check is missing or wrong.
- **Fork-choice head, justified or finalized checkpoint differs:** weights, tie-breaking, safe
  target or attestation timing (which interval promotes votes).
- **Encoding or hash-tree-root mismatch:** struct tags or field order in `internal/lean/types`;
  regenerate with `make lean-sszgen` after fixing the tags.

## Phase 3: Rule out the fixtures before changing gean

1. Are the fixtures the pinned file? `leanSpec/fixtures/.sha256-<hash>` must match
   `LEAN_SPEC_FIXTURES_SHA256`. If not, delete `leanSpec/fixtures` and run `make lean-spectest` again.
2. Did the spec change on purpose? In a leanSpec clone, read the change for the rule involved:
   `git log -p <old>..<new> -- src/lean_spec/` (the spec lives under
   `src/lean_spec/spec/`, fork logic under `forks/<fork>/`).
3. Is the test itself wrong? Search leanSpec's issues and pull requests for the test's name before
   concluding gean is.

## Phase 4: Find the cause in gean

- Read the spec function the fixture exercises, then gean's counterpart, side by side.
- Reproduce with the smallest case: one fixture, one block or one step.
- Read the history of the gean code you'd change (`git log -p -- <file>`); don't reintroduce
  behaviour removed on purpose just to pass a test.

## Phase 5: Fix and verify

1. Fix gean so it matches the spec, with a comment only where the reason isn't obvious.
2. Run the failing case, then the whole test, then everything:

   ```bash
   cd testing/spectest/lean && go test -tags=spectests -count=1 -run 'TestSpec/<directory>/<case>'
   make lean-spectest
   make test
   ```

3. Run the [`prep-commit`](../prep-commit/SKILL.md) skill before committing. If the fix changes
   consensus behaviour, also run `make lean-e2e`.

Report which phase found the cause, what was wrong (gean, harness or fixture), and the fix.
