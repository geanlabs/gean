---
name: lean-review
description: Review a branch or PR's diff against main and report opportunities to subtract — simplify, delete, and reduce the number of concepts a reader has to hold in their head. Use when users want to (1) review a branch before merge for bloat, (2) find dead code or premature abstractions, (3) identify duplication of existing utilities, (4) audit AI-generated code for over-engineering, or (5) reduce LOC without changing behavior. Report-only by default; only applies fixes when the user explicitly says so.
---

# /lean-review — Subtract over add

Review the current branch's diff against a base branch (default `main`) and
report opportunities to simplify, delete, or consolidate. The goal is to lower
the number of concepts a reader has to hold in their head, not just to shrink
LOC. AI-generated code tends toward additive: extra layers, validation that
can't fire, helper functions used once, comments restating the code. This skill
forces a "what can come out?" pass before merge.

This skill is **report-only by default**. After the report is produced, the
user may say "apply #2, #5, #7" or "fix the cosmetic ones" and the skill will
apply only those specific findings, never the consensus-critical ones without
explicit per-finding confirmation.

## Non-negotiable rules (gean philosophy)

These hold regardless of category or risk class. Treat a violation of any of
them as a finding in its own right.

1. **Reduction over addition.** Never propose adding code. The only output is
   "what comes out." If the honest fix is "add a helper," say so as a note and
   stop — don't author it.
2. **Simplicity over cleverness.** No generics, reflection, deep interface
   hierarchies, single-impl interfaces, or premature flags. Buffer caps/limits
   stay package-level `const` sized for devnet-4 ("promote to a flag *when* a
   deployment needs it," not before).
3. **SSZ `*_encoding.go` is generated, never hand-authored.** Flag any
   hand-written or hand-edited `*_encoding.go`, or any by-hand SSZ
   encode/decode that `make sszgen` would produce. The fix is always: edit the
   struct + tags, run `make sszgen`. If `make sszgen` fails, STOP and report
   the error — never suggest writing/patching the file by hand as a fallback.
4. **No foreign-client comments.** When code mirrors another client
   (ethlambda, zeam) or leanSpec, its comments must be peculiar to gean and as
   short as necessary. Flag any comment copied from, or naming, another client
   ("from ethlambda", "see zeam", etc.). A short leanSpec *function-name*
   cross-ref is allowed; provenance prose and other-client names are not.
5. **Concept count, not LOC.** A finding is real only when it removes a named
   thing a reader must hold in their head. Collapsing two lines into one is not
   a finding.
6. **Spec structure is load-bearing.** Code that mirrors leanSpec function
   names/control-flow is not "duplication" — leave it, tag report-only.

## Quick start

```bash
# Review current branch vs main
.claude/skills/lean-review/scripts/collect-diff.sh

# Review vs a different base
.claude/skills/lean-review/scripts/collect-diff.sh devnet-4
```

Then walk the changed files and produce the structured report below.

## Workflow

### 1. Collect the diff

Run `scripts/collect-diff.sh [base]` to get a structured overview of what
changed: files, line counts per file, new files, biggest additions, total LOC
delta. This bounds the scope — review **only what this branch added**, not
the whole repo. If you start suggesting changes to code that already existed
on the base branch, you're out of scope.

### 2. Filter out don't-touch paths

Drop any file matching `references/dont-touch.md` before analysis. The most
common ones: `*_encoding.go` (generated), `xmss/rust/` (different review
process), `internal/specfixtures/*.go` (mirrors JSON shape), spec-mirroring function
bodies in `internal/statetransition/` (structure must match leanSpec for
auditability). If the entire diff is don't-touch, say so and stop.

### 3. Read each remaining file and classify additions

For each addition (a new function, a new branch, a new helper, a new struct
field), classify it into one of the six categories from
`references/finding-categories.md`:

| Category | What it looks like |
|---|---|
| `dead-code` | Unused function/method, unreachable branch, variable written never read, exported symbol no caller imports |
| `premature-abstraction` | Helper called exactly once, interface with one implementor, generic that has only one instantiation, wrapper that adds no behavior |
| `defensive-bloat` | Validation at an internal boundary already guaranteed by the caller, nil-check on a value the type system says can't be nil, error path that can't be reached |
| `duplicates-existing-util` | New code that could call a function already in this package (or `internal/types/`, `internal/storage/`, `logger/`, etc.) |
| `comment-bloat` | Comments that restate the code, "added for X" / "used by Y" rot, stale TODOs, multi-line docstrings on obvious functions |
| `over-validated-boundary` | Treating an internal caller's input as if it were external user input |

For each finding, before reporting it, you MUST:

- **Grep the package** (and `internal/types/`, `internal/storage/`, `logger/`) for utilities
  that already do what the new code does. The single highest-value finding
  type is "this could reuse `X` in `path/file.go:LINE`" — humans miss these
  most often.
- **Note test coverage**. Run `grep -l <symbol>` in the relevant test
  directory. A finding backed by "removed code, all callers covered by
  tests" is much stronger than pure static reasoning.

### 4. Assess risk class per finding

| Risk class | Definition | What's allowed |
|---|---|---|
| `cosmetic` | Comments, naming, formatting, log strings. Cannot affect runtime behavior. | Auto-apply when user requests `--fix`. |
| `structural` | Code refactor; behavior must be preserved, tests must re-pass. Examples: inlining a one-use helper, deleting an unused field, collapsing two branches. | Apply on explicit per-finding approval. Re-run tests after each apply. |
| `consensus-critical` | Anything in `internal/statetransition/`, `internal/forkchoice/`, `internal/node/` (engine/tick/gossip/head/import — the single-writer loop), `internal/pending/`, `internal/store/consensus_store.go`, `internal/types/` (non-encoding), `internal/p2p/host.go`, `internal/p2p/reqresp.go`, or the XMSS Go binding. | **Report only.** Never apply without explicit per-finding human review, even on `--fix`. |

When in doubt, escalate the risk class. A wrong "structural" call on
consensus code is much worse than a verbose report.

### 5. Cross-reference spec compliance

For findings inside `internal/statetransition/` or `internal/types/`, check whether the
"redundant" code mirrors a named leanSpec function. Code that looks
duplicated may be intentionally mirroring two separate spec functions to
preserve auditability. See `.claude/skills/spec-compliant/references/spec-gean-mapping.md`
for the mapping. If a finding would diverge gean from spec structure, tag it
`spec-divergence-risk` and downgrade to report-only regardless of category.

### 6. Generate the report

Output in this structure. One section per category, findings sorted by LOC
saved descending within each category. End with the summary table.

```markdown
## /lean-review report — <branch> vs <base>

**Scope:** N files changed (+A / -B LOC), M files reviewed after don't-touch filter

### dead-code

#### #1 — `internal/syncer/backfill.go:142-168` (−27 LOC) — `structural`
**Finding:** `markStaleRoots` is exported but only called inside the package,
and its only caller `pruneStaleRoots` was deleted in commit abc1234. No
external imports.

**Suggested change:** Delete `markStaleRoots` and its test
`TestMarkStaleRoots`.

**Why safe:** No callers outside the file. `go vet ./...` and
`make test` should pass unchanged.

**Test coverage:** `internal/syncer/backfill_test.go:88` (TestMarkStaleRoots) — also delete.

---

#### #2 — `internal/p2p/peers.go:55-58` (−4 LOC) — `cosmetic`
**Finding:** Unreachable branch — `if peer == nil` after `peer, ok :=
m[id]; !ok { return }` two lines above.

**Suggested change:** Delete the nil-check block.

**Why safe:** Control flow guarantees `peer != nil` at this point.

**Test coverage:** N/A (dead branch).

---

### premature-abstraction

#### #3 — `internal/node/duties.go:204-218` (−15 LOC) — `consensus-critical`
**Finding:** `wrapAttestationSigner` is a 14-line wrapper around
`keys.SignAttestation` with a single call site at `duties.go:312`. Adds
no behavior.

**Suggested change:** Inline the call. See report-only note below.

**Why safe (proposed):** Wrapper is pure passthrough; inlining is a
behavior-preserving refactor.

**Test coverage:** `internal/node/duties_test.go:177` covers the call site.

> **Report-only.** Consensus-critical path. Even though the refactor is
> mechanical, the wrapper may exist to mark a future signature-policy
> insertion point. Human must confirm intent before inlining.

---

### duplicates-existing-util

#### #4 — `internal/node/gossip.go:88-104` (−14 LOC) — `structural`
**Finding:** New helper `shortRoot` duplicates
`internal/types/helpers.go:25 ShortRoot`, same signature, same body.

**Suggested change:** Delete the local helper, import + call
`types.ShortRoot`.

**Why safe:** Identical implementation; same test coverage flows through.

**Test coverage:** `internal/types/helpers_test.go` covers ShortRoot.

---

### defensive-bloat

(no findings)

### comment-bloat

#### #5 — `internal/storage/pebble.go:201-209` (−8 LOC) — `cosmetic`
**Finding:** 8-line comment explaining what `Get` does, when the function
signature `Get(table Table, key []byte) ([]byte, error)` is
self-documenting. Comment also includes "added for issue #142" which is PR
context, not code context.

**Suggested change:** Delete the comment.

**Why safe:** Pure comment removal, no runtime effect.

---

### over-validated-boundary

(no findings)

### Summary

| Category | Cosmetic | Structural | Consensus-critical | LOC saved |
|---|---|---|---|---|
| dead-code | 1 | 1 | 0 | 31 |
| premature-abstraction | 0 | 0 | 1 | 15 |
| duplicates-existing-util | 0 | 1 | 0 | 14 |
| defensive-bloat | 0 | 0 | 0 | 0 |
| comment-bloat | 1 | 0 | 0 | 8 |
| over-validated-boundary | 0 | 0 | 0 | 0 |
| **Total** | **2** | **2** | **1** | **68** |

**Top concepts removed:** one helper duplicating an existing utility, one
exported-but-unused function, one wrapper around a single call.

**Next step:** Reply with finding IDs to apply (e.g. "apply #1 #2 #4 #5") or
"apply all cosmetic" / "apply all structural". Consensus-critical findings
(#3) require explicit per-finding approval and will not be auto-applied.
```

## Fix mode

When the user replies with `apply #N` or similar after the report:

1. Apply only the requested findings, in order.
2. For each finding, run the smallest test scope that covers it
   (`go test ./<package>/`). For consensus-critical findings, also run
   `make test`. For findings in `internal/types/`, also run `make sszgen` first if a
   struct changed, then `make test`.
3. Commit each applied finding as its own commit with the finding ID and a
   one-line summary, so each is trivially revertible:
   `git commit -m "lean-review #4: inline shortRoot into types.ShortRoot"`
4. If any test fails: stop, revert the last commit, report which finding
   caused the failure, do not continue to the next finding.
5. Refuse `apply #N` for any consensus-critical finding unless the user
   also says "I've reviewed this" or equivalent. Print the finding and ask
   for explicit confirmation.

## What this skill does NOT do

- **Does not propose adding code.** If the right fix is "this needs a new
  helper," that's outside scope — flag it as a comment in the report but
  don't suggest the addition.
- **Does not reformat or restyle.** `gofmt` and `golint` handle that.
- **Does not enforce coding standards.** It looks for subtraction
  opportunities, not style violations.
- **Does not benchmark.** Performance suggestions belong in a different
  review pass.
- **Does not touch files outside the diff.** Even if you spot something on
  the base branch that's bloated, it's out of scope for this branch's
  review.

## Scripts

| Script | Description |
|---|---|
| `scripts/collect-diff.sh [base]` | Gather diff stats vs base branch (default `main`): changed files with line counts, new files, biggest additions, total delta |

## References

- `references/finding-categories.md` — full examples and counter-examples for each of the six categories
- `references/dont-touch.md` — paths and patterns to skip entirely
- `.claude/skills/spec-compliant/references/spec-gean-mapping.md` — cross-reference when deciding whether code "duplicates" something or mirrors the spec
