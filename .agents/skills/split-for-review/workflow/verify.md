<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Verify

*Goal:* prove, not argue, that the staged unit builds and behaves on its own. Cite this module when a unit was presented as reviewable without evidence, or when "it should compile" turned out to be wrong.

## VERIFY/EXTRACT_THE_INDEX_TREE
*Ensures:* what gets verified is the unit as it will be committed — not the worktree, which contains every later unit too.

The worktree holds the final state, so building it proves nothing about a unit. `split.sh verify` materialises the index as a standalone tree and runs the checks there.

Cost: the extract and its build directory persist across units. Only files that differ from the previous run are rewritten, so only the changed packages and their dependents rebuild. Each verdict is recorded by tree, so re-verifying an identical tree returns the recorded one. The last unit is the whole change, so its verdict is the baseline's.

What the extract is: exactly the index, so a new file not yet staged is correctly absent; throwaway, so nothing done in it can affect the repo; and a one-commit git repository, because build scripts that read version-control metadata do not compile in a bare copy.

What it lacks: gitignored fixtures (spec vectors, captured traces). Suites that need them usually *skip* rather than fail, so `split.sh verify` links them in from the real repo (`SPLIT_LINK` overrides which).

*Failure modes:* the worktree is checked and every unit "passes" because the later units supply the missing pieces; spec-vector suites silently skip in the extract and a shrunken run is reported as green.

## VERIFY/TWO_SPEED
*Ensures:* design iteration on a staged unit stays fast, and the verdict handed back is still the full one.

While the user is still reshaping a unit, check each edit with the scoped, fast run (`split.sh check ./<package>/...`). Run the authoritative `split.sh verify` once the design settles, before the unit is handed back as ready. Report the final verdict, not every intermediate run.

*Failure modes:* a two-minute full suite after every one-line rename; the fast run's pass reported as the verdict and a build-tag-gated failure ships.

## VERIFY/FULL_STRICTNESS
*Ensures:* the unit is held to the same bar as the repo's CI, so no unit defers work to a later one.

Run the repo's own recipes through `split.sh verify`; never substitute hand-rolled flags. They differ in what they compile, which build tags they set and which lints they apply, so a substitute produces failures that belong to nobody and passes that mean nothing. Test code may be compiled only by the test run, so a linter pass alone never makes a unit clean.

`-D warnings` is load-bearing for splitting specifically, because the two most common artefacts of a mid-series state are exactly warnings: an item added in this unit but not used until the next one, and an import left behind by a partial edit. Both mean the boundary is wrong — pull the first use into this unit, or move the definition to the unit that uses it.

*Failure modes:* a unit lands with a dead-code warning because its only caller is in the next unit; hand-rolled flags flag pre-existing lints the repo never checks, and the unit is re-cut to satisfy a standard that does not exist.

## VERIFY/READ_THE_STATUS_NOT_THE_TEXT
*Ensures:* a "clean" verdict is derived from an exit code, not from a grep that could not have matched anyway.

Judge every check by its exit status. Three traps, each a silent false pass:

- **Piped exit codes.** `suite | tail` reports the pager's status, not the suite's. Redirect to a file, check `$?`, then read the file.
- **`set -e` in the harness.** `cmd; rc=$?` under `set -euo pipefail` dies on the failing command before it can say which check failed. Use `if ! cmd; then` or `cmd || rc=$?` so the summary always prints.
- **ANSI-coloured output.** `grep -E "^(error|warning)"` matches nothing when the line begins with a colour escape. Strip escapes (`sed 's/\x1b\[[0-9;]*m//g'`), and never let the absence of grep output stand in for success.

State in the report what was run and what its status was.

*Failure modes:* a unit is reported clean on the strength of an empty grep.

## VERIFY/DISTINGUISH_PREEXISTING
*Ensures:* a failure is attributed correctly, and a shrunken run is not mistaken for a clean one.

Compare every result against the baseline from `BASE/WHOLE_DIFF_GREEN_FIRST`:

- A failure on the baseline's list is reported as pre-existing, named, and not fixed inside the split. A failure not on it belongs to the unit and blocks it.
- The test count must match the baseline, less the tests later units add. A materially lower count means a suite silently skipped.

Re-run a single suspicious failure before drawing a conclusion; a flaky test will otherwise send the split in a circle.

*Failure modes:* a pre-existing red test is "fixed" inside a split unit; a genuine regression is dismissed as pre-existing because the baseline was never recorded.
