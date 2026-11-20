<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Base

*Goal:* establish exactly what is being split, prove it is green before carving it up, and keep a copy of it that later steps author intermediates from. Cite this module when a unit turned out to contain someone else's change, when an edit vanished mid-split, or when a unit's failure could not be told apart from a pre-existing failure.

## BASE/PIN_AND_DETECT_DRIFT
*Ensures:* the split is measured against one known commit, and a base that moves unexpectedly is caught immediately rather than surfacing as a mystery compile error three units later.

Record `git rev-parse HEAD` and the branch name before anything else, and state both to the user. The pin is the tip of the series so far: a unit the user commits on top of it is an expected move. Before staging each subsequent unit, re-check (`split.sh drift`).

Any other move — a pull, a checkout, a rebase, a teammate's push landing in the same worktree — silently reverts locally-modified files that differ between the two commits, so an edit that existed when the split was planned can be absent by the time its unit is staged. If that happened: stop, say so, name the files whose modifications were lost by diffing the snapshot against the worktree, and re-establish the snapshot before continuing. Do not re-plan the split on the assumption that the diff is unchanged.

*Failure modes:* a re-export or a `use` line that was part of the change is missing and gets re-added inside the wrong unit; the split is presented as "vs main" when main has advanced and half the diff is now someone else's merged work.

## BASE/START_FROM_LATEST_DEFAULT
*Ensures:* the split is cut on the branch it will merge into, not on a branch whose PR already landed.

Right after pinning, fetch, then run `split.sh stale`. It reports whether the pinned branch's PR is merged or closed, and how far `origin/<default>` has moved. It also lists the changed files that upstream also touched since the pin. When the branch is stale, or the default branch moved, propose moving the change onto the latest `origin/<default>`. Give the branch name for each PR (`PRS/SUGGEST_BRANCH_NAMES`) and the overlap list. Move only on the user's go, using the steps in `PRS/AFTER_LOWER_PR_MERGES` (stash, `--no-track` branch, pop, byte-compare every changed file). Then re-pin and re-take the baseline.

*Failure modes:* units are cut and verified on a branch whose PR squash-merged a week ago, so every unit has to be re-staged on main; a move absorbs upstream hunks into the change because the overlap list was never checked.

## BASE/WHOLE_DIFF_GREEN_FIRST
*Ensures:* per-unit verification failures mean something, because the union was known-good.

Before splitting, run `split.sh baseline`: the same checks as `split.sh verify`, on the whole change. It runs in the same extract and build directory the units use, so it also warms them for the first unit. Record the result — pass/fail per check, the test count, and every pre-existing failure by name — and tell the user. That record is the baseline `VERIFY/DISTINGUISH_PREEXISTING` compares against; how to read a result without being misled is `VERIFY/READ_THE_STATUS_NOT_THE_TEXT`.

*Failure modes:* unit 3's verification "fails" on a test that was already red at the base and the split is blamed.

## BASE/SNAPSHOT_THE_FINAL_STATE
*Ensures:* intermediates can be authored, and the worktree never has to be reverted to produce them.

Copy every changed and untracked file of the change into a snapshot directory outside the repo (`split.sh snapshot`; `SPLIT_EXCLUDE` keeps unrelated untracked paths out). This snapshot is the authority for what each file must look like when the last unit lands (`STAGE/AUTHOR_SPLIT_FILES`, `STAGE/UNION_IS_THE_DIFF`).

The snapshot starts the split from an empty index. Take it before the first unit is staged, or re-take it only when the index is empty.

*Failure modes:* the last unit leaves a file subtly different from what the user wrote, because the intermediate was authored from memory rather than diffed against the snapshot.
