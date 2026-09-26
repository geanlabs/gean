---
name: lean-review
description: Review a branch or PR's diff against main and report opportunities to subtract — simplify, delete, and reduce the number of concepts a reader has to hold in their head. Use when users want to (1) review a branch before merge for bloat, (2) find dead code or premature abstractions, (3) identify duplication of existing utilities, (4) audit AI-generated code for over-engineering, or (5) reduce LOC without changing behavior. Report-only by default; only applies fixes when the user explicitly says so.
---

# /lean-review — Subtract over add

Find what can come out of a change before merge: dead code, premature
abstractions, defensive filler, duplicated utilities, comment bloat.

## Rules

The review standard is the Principles and Code Style sections of `AGENTS.md`
(subtract before adding, no single-implementation interfaces, no defensive
filler, generated SSZ code, no other-client names in comments, and so on). A
violation of any of them in the diff is a finding. On top of those:

1. **Report-only by default.** Apply nothing until the user names findings to
   apply (see Fix mode). Never propose adding code; if the honest fix is "add a
   helper," note it and stop.
2. **Concept count, not LOC.** A finding is real only when it removes a named
   thing (function, type, field, branch) a reader must hold in their head.
   Collapsing two lines into one is not a finding.
3. **Spec structure is load-bearing.** Code that mirrors leanSpec function
   names or control flow is not duplication. Leave it; tag any finding that
   would diverge from spec structure `spec-divergence-risk`, report-only.
4. **Scope is what the change added.** Review only files and lines in the
   collected diff, never code that already existed on the base.

## Workflow

### 1. Collect the scope

```bash
.agents/skills/lean-review/scripts/collect-diff.sh [base]   # default base: main
```

It lists every file changed relative to `<base>`, labeled by source:
`committed` (`<base>...HEAD`), `staged`, `unstaged`, or `untracked`, with line
counts, hard-skip matches, and the commit log. After editing the script, run
`scripts/test-collect-diff.sh`.

### 2. Drop don't-touch paths

Apply `references/dont-touch.md`: drop hard-skip files; keep soft-skip findings
but tag them `consensus-critical`. If nothing reviewable remains, say so and stop.

### 3. Classify each addition

Put each finding in one category from `references/finding-categories.md`:

| Category | What it looks like |
|---|---|
| `dead-code` | No callers or readers, unreachable branch, unused export |
| `premature-abstraction` | Helper used once, single-implementation interface, passthrough wrapper |
| `defensive-bloat` | Check for a case proven impossible, error path that cannot be reached |
| `duplicates-existing-util` | Reimplements something already in the repo |
| `comment-bloat` | Restates the code, carries change history, names another client |
| `over-validated-boundary` | Treats trusted internal input as untrusted |

Before reporting a finding:

- Grep the package, `internal/types/`, `internal/storage/`, and `internal/logger/`
  for an existing utility. "This could reuse `X` at `path:line`" is the
  highest-value finding.
- Note which tests cover the code (`grep -rl <symbol> --include='*_test.go'`).
- **Removing a guard needs proof it is unreachable.** Trace input boundaries,
  every caller, and how the state is constructed. A successful lookup or a
  type is not proof: a Go map can hold a nil pointer, so
  `peer, ok := m[id]; if !ok { return }` does not make `peer != nil`. Without
  that proof it is not a finding. With it, the finding is `structural` or
  `consensus-critical`, never `cosmetic`.

### 4. Assign a risk class

| Class | Definition | Fix mode |
|---|---|---|
| `cosmetic` | Comments, naming, log strings. Cannot change runtime behavior. | Apply when requested. |
| `structural` | Behavior-preserving code change (inline a helper, delete an unused field, remove a proven-unreachable guard). | Apply when requested; tests must pass. |
| `consensus-critical` | Any soft-skip path in `references/dont-touch.md`. | Report only unless the user confirms that specific finding after review. |

When in doubt, escalate. A verbose report costs attention; a wrong
`structural` call on consensus code can fork the chain.

### 5. Report

Group findings by category, sorted by LOC saved; omit empty categories.

```markdown
## /lean-review — <branch> vs <base>

**Scope:** N files (+A / -B), M reviewed after don't-touch filter

### <category>

#### #1 — `path/file.go:L1-L2` (−N LOC) — `<risk class>`
**Finding:** what the concept is and why it is removable (with proof).
**Change:** what to delete or reuse.
**Tests:** covering tests, or none.

### Summary

| Category | Cosmetic | Structural | Consensus-critical | LOC saved |
|---|---|---|---|---|

**Next step:** reply with finding IDs to apply (e.g. "apply #1 #3").
```

## Fix mode

When the user names findings to apply:

1. Apply only those findings, in order, to the working tree. Do not commit,
   stage, or revert commits; the user commits (conventional commits, per
   `AGENTS.md`).
2. For a `consensus-critical` finding, show it and wait for explicit
   confirmation of that finding before editing.
3. After each finding, run the smallest covering tests (`go test ./<pkg>/`).
   Also run `make test` for consensus-critical findings, and `make sszgen`
   first if an SSZ struct changed (plus `make test-spec`).
4. If a test fails, undo that finding's own edits, stop, and report which
   finding failed and why. Leave earlier applied findings in place.

## Out of scope

Adding code, formatting or lint (`make fmt`, `make lint`), performance
(benchmark in a separate pass), and code outside the diff.

## References

- `references/finding-categories.md` — examples, required proof, and
  exceptions per category
- `references/dont-touch.md` — hard-skip and consensus-critical paths
- `scripts/collect-diff.sh [base]`, `scripts/test-collect-diff.sh`
