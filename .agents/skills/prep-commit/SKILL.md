---
name: prep-commit
description: Get a gean change ready to commit — report cleanups, apply the ones the user approves, then format, lint, build and test it. Quality only; use the review skill for bugs. Also use when asked to "clean up", "de-slop" or check a change.
---

# Prep commit

You are improving the quality of the changed code, not hunting for bugs. Review it for reuse,
simplification, efficiency, altitude, unused code, test bloat, AI slop and Go idioms. Do not look
for correctness bugs — that is what the [`review`](../review/SKILL.md) skill is for.

The bar is a careful human reviewer on this codebase: every line in the diff should earn its
place, and the code should read as if a person who knows gean wrote it. When a finding has
several fixes, prefer the one that leaves less code: a good run usually shrinks the diff.

## Phase 0 — Gather the diff

Run `git diff @{upstream}...HEAD` (or `git diff main...HEAD` / `git diff HEAD~1` if there's no
upstream) to get the unified diff under review. If there are uncommitted changes, or the range
diff is empty, also run `git diff HEAD` and include the working-tree changes in scope — the review
often runs before the commit. If a PR number, branch name, or file path was passed as an argument,
review that target instead. Treat this diff as the review scope.

Skip generated files (`internal/lean/types/*_encoding.go`) and anything `make lint` already
enforces.

## Phase 1 — Review (8 cleanup agents in parallel)

Launch **8 independent review agents** via the Agent tool, all in a single message so they run
concurrently. Pass each agent the diff, `AGENTS.md`, the `.agents/rules/` files for the areas the
diff touches, and one of the eight angles. Each returns its findings with `file`, `line`, a
one-line `summary`, and the concrete cost (what is duplicated, wasted, unused, or harder to
maintain).

### 1. Reuse

Flag new code that re-implements something the codebase already has — Grep shared/utility
modules and files adjacent to the change, and name the existing helper to call instead. In gean,
look first in `internal/core` (clock, lifecycle, logger, metrics, p2p, pending, protoarray,
storage, httpapi) and in the sibling files of the package being changed.

### 2. Simplification

Flag unnecessary complexity the diff adds: redundant or derivable state, copy-paste with slight
variation, deep nesting, dead code left behind. Name the simpler form that does the same job.

Also flag over-engineering:

- An interface with one implementation, a wrapper or helper with one caller, a type alias used
  once, a forwarding function that only calls another: inline it.
- An abstraction, option or config field added "for later" with no caller now. Three similar lines
  are better than a premature abstraction.
- A fallback, retry or nil check for a case that cannot happen; an invariant re-checked after
  something upstream already enforces it. If correctness depends on it, the code should say where
  it is enforced, once.
- A cache, pool, goroutine or channel with no measured need.
- A layer whose reason can't be stated in one sentence.
- Code a reader can't follow in one pass: a function mixing levels of detail, control flow that
  is hard to trace.

### 3. Efficiency

Flag wasted work the diff introduces: redundant computation or repeated I/O, independent
operations run sequentially, blocking work added to startup or hot paths. Also flag long-lived
objects built from closures or captured environments — they keep the entire enclosing scope alive
for the object's lifetime (a memory leak when that scope holds large values); prefer a struct that
copies only the fields it needs. Name the cheaper alternative.

In gean, hot paths include gossip validation, block import, the engine's dispatch loop, fork
choice and SSZ encoding; blocking or XMSS work does not belong on the dispatch loop.

### 4. Altitude

Check that each change fixes the root cause at the right depth rather than patching a symptom
with a fragile bandaid. Special cases layered on shared infrastructure are a sign the fix isn't
deep enough — prefer the simpler, more general change to the underlying mechanism over adding
special cases, and name that change.

In gean, a lean-specific special case inside `internal/core` is always the wrong altitude: it
belongs in `internal/lean`.

### 5. Unused and speculative code

Flag code the diff adds without a caller in the same change, and code it leaves behind:

- Functions, methods, parameters, struct fields, constants, exported identifiers, CLI flags,
  config fields, metrics or log lines nothing uses. Remove an unused parameter; don't keep it as `_`.
- Code made unused by the change: when something is removed or replaced, what only it used goes
  too.
- Commented-out code, and compatibility shims, aliases or fallbacks for old flags or formats
  (gean is devnet-only and keeps no backward compatibility).

### 6. Tests

Flag test code that costs more than it checks:

- Tests that differ only in inputs and expected outputs: merge them into one table-driven test.
  Before a new test, check whether an existing table can take another case.
- Cases that exercise the same path and would fail for the same reason as another case: keep one.
- Tests of trivial code (getters, constructors, constant values) or of Go and the standard library.
- Tests that pin implementation details rather than behaviour, so any refactor breaks them.
- Setup copied between tests: move it into a helper next to the tests that use it.
- `time.Sleep` to wait for something: wait on the condition instead.
- Real XMSS work where `-tags fakecrypto` or an existing fixture would do.

Do not flag a test because it is long; flag it because something in it is duplicated or proves
nothing.

### 7. AI slop and comments

Flag text a careful human wouldn't have written:

- Comments that restate the code, narrate the change ("now we…", "updated to…", "as requested",
  "new:"), or repeat a function's name as its doc. The default comment count on clear, well-named
  code is zero; a comment earns its place by saying why, an invariant, or a spec reference.
- Uncertainty or generation residue: "not sure", "maybe this should", "AI generated", a bare
  `TODO` with nothing actionable.
- References to plans, phases or the session in code, comments, commit messages or config.
- Docs much longer than the code they describe; a preamble that paraphrases the type.
- Defensive boilerplate: logging and re-returning every error at every level, redundant nil checks,
  validation already done by the caller.
- Vague names (`data`, `info`, `helper`, `manager`, `util`) where a specific one exists; a comment
  explaining a name that should be renamed instead.
- Drive-by reformatting, renames or comment rewrites in code the change doesn't otherwise touch.

### 8. Go idioms

Flag Go a careful Go reviewer would rewrite, skipping what `staticcheck` already reports:

- Errors wrapped without `%w`, or compared with `==` instead of `errors.Is` or `errors.As`.
- A `context.Context` that isn't the first parameter, or is stored in a struct for one call's work;
  a long-lived service keeping its lifetime context, like the syncer, is fine.
- Cleanup not done with `defer`, so an early return skips it.
- An interface declared by the package that implements it rather than the one that uses it.
- A field accessor named `GetX`; lookups by key like `GetBlockHeader(root)` are gean's style.
- Names that stutter with their package (`store.StoreConfig`).

## Phase 2 — Report and wait

Wait for all eight agents to complete and dedup findings that point at the same line or
mechanism. Check the findings as the [`verify-report`](../verify-report/SKILL.md) skill describes,
without its separate report, and correct them before the user sees them.

Report what remains, numbered, most important first, each with the fix you propose. Mark a finding
whose fix would change intended behavior or reach well outside the diff with
"(would skip: <reason>)". Plain sentences, the way a colleague would say it. If there are no
findings, say the change is already clean and go to Phase 3.

Then stop, and apply exactly the findings the user approves.

## Phase 3 — Checks

Run these from the repository root, in order. If a step other than formatting fails, stop, report
its cause and the fix you propose, and wait for the go-ahead; then run that step again.

1. **Formatting.** If the next command lists any files, run `make fmt` and check again:

   ```bash
   gofmt -l $(git ls-files '*.go')
   ```

2. **Lint.** Any finding fails it:

   ```bash
   make lint
   ```

3. **Build:**

   ```bash
   make build
   ```

4. **Tests for the changed packages.** Turn the changed `.go` files into packages and test them:

   ```bash
   pkgs=$({ git diff --name-only HEAD -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } |
     xargs -r -n1 dirname | sort -u | while read -r d; do [ -d "$d" ] && echo "./$d"; done)
   if [ -n "$pkgs" ]; then go test $pkgs -count=1; else echo "no Go packages changed"; fi
   ```

   Then add what the change touches:

   | Changed | Also run |
   |---|---|
   | `internal/lean/transition`, `forkchoice`, `types` or `api` | `make lean-spectest` |
   | `internal/lean/node`, `importer`, `syncer`, `aggregation`, `p2p`, `internal/core/p2p` or `lean.Run` | `make lean-e2e` |
   | `internal/core/crypto/xmss` or its Rust code | `make lean-test-ffi` (slow) |
   | Anything shared by many packages | `make test` |

## Phase 4 — Record

Record the change as prepared. The Claude Code commit hook (`.claude/settings.json`) denies
`git commit` until the recorded id matches the current change, so any later edit needs another
run:

```bash
.agents/skills/prep-commit/change-id.sh > "$(git rev-parse --git-dir)/prep-commit-done"
```

Finish with what was applied, what was left, and which checks ran and passed.
