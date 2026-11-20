<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: PRs

*Goal:* group ordered units into pull requests a team can review, keep each PR on its own branch, and keep the series coherent as PRs merge. Cite this module when a reviewer had to open more PRs than the change warranted, when a PR could not be reviewed without another one, or when a series fell apart after its first PR merged.

This module owns PR boundaries, branches and the PR lifecycle. Unit boundaries belong to `workflow/units.md`; their order to `workflow/order.md`; how any of it is shown to the user to `workflow/report.md`.

## PRS/ONE_THEME_PER_PR
*Ensures:* each PR can be approved or rejected for one reason a teammate recognises.

A PR is a run of consecutive units sharing one theme a reviewer can name in a title. Units stay the commits inside it, so the fine-grained review steps survive.

Default to one PR. Split into several only when the user asks, or when the themes would have different reviewers or could land at different times. Never make a PR per unit.

*Failure modes:* nine PRs for nine units, so teammates review plumbing out of context.

## PRS/NAMED_DEPENDENCY_EDGES
*Ensures:* the stack order is justified by the code, not by the author's memory.

For every PR after the first, name what it needs from an earlier PR by the exact symbol: a type, a field, a function, a table. "C needs B's `queue_slot_counters` and `NodeMeta.version`" is an edge; "C builds on B" is not. A PR with no edge to its predecessor is independent: base it on the default branch, so it can land alone.

*Failure modes:* a PR stacked on another it does not use, so it waits for no reason.

## PRS/BRANCHES
*Ensures:* each PR's branch holds exactly its own units, and nothing lands on the default branch by accident.

The user creates branches, or tells the skill to. Before the first unit of each PR is staged, the skill proposes its name (`PRS/SUGGEST_BRANCH_NAMES`) and says which commit it must start from — the previous PR's last unit for a dependent PR, the default branch for an independent one.

If HEAD is the default branch, say so before the first unit is staged. If units have already landed there and nothing was pushed, the repair is three pointer moves the user runs: `git branch <name>` at HEAD, `git checkout <name>`, then `git branch -f <default> <upstream>`. That keeps the commits and their SHAs and leaves the worktree untouched; never `reset --hard`.

*Failure modes:* two PRs' units on one branch, so the second PR's diff shows the first; a series committed straight onto the default branch.

## PRS/SUGGEST_BRANCH_NAMES
*Ensures:* the user picks branch names from a concrete proposal instead of inventing them mid-split.

Propose one branch per PR in the plan (`REPORT/PLAN_FIRST`) and whenever the series moves onto the default branch. Derive each name, never guess it:

- **Prefix:** the one the user's recent branches share (`git branch --sort=-committerdate`, e.g. `alexey/`).
- **Stem:** the PR's theme in the repo's existing style (`snake_case` here), naming the change, not the process: `fix_page_pool`, not `split_part_1`.
- **Free:** absent locally and on origin (`git branch -a --list '*<stem>*'`). If taken, say by what, and suggest another.
- **Start point:** `origin/<default>` for an independent PR, the previous PR's branch for a stacked one (`PRS/NAMED_DEPENDENCY_EDGES`).

Give the exact creation command: `git checkout --no-track -b <name> <start>`. The user may accept, rename, or tell the skill to create it. A name the user gives always wins.

*Failure modes:* the user is asked "what branch?" with no proposal; a suggested name already exists on origin and the first push collides; a stacked PR's branch is cut from main and its diff misses the lower PR.

## PRS/AFTER_LOWER_PR_MERGES
*Ensures:* when a lower PR merges, the rest of the series moves onto the merged branch without losing an edit or absorbing upstream changes into a unit.

This is one of the two exceptions to `STAGE/INDEX_ONLY`, and it runs only when the user asks for it. Each step is checked before the next:

1. Fetch, and confirm the merge commit is on the fetched branch.
2. List the files the new upstream commits touch that are also in the change; those are the files the move will merge.
3. `git stash push -u`, check out the target, `git merge --ff-only` to the fetched tip, `git stash pop`. A conflict stops the move and is reported, not resolved silently. A new branch for the next PR is created with `git checkout --no-track -b <name> origin/<default>`: without `--no-track` it tracks the default branch, so `push.autoSetupRemote` never applies and every first push needs `-u`. When the fetched tip's tree equals the local branch's tree, the index and worktree carry over without a stash.
4. Compare every changed file against the old snapshot: only step 2's files may differ, and only by upstream's hunks.
5. With the index empty, take a new snapshot (`BASE/SNAPSHOT_THE_FINAL_STATE`) and a new baseline (`BASE/WHOLE_DIFF_GREEN_FIRST`); regenerate every remaining intermediate that depends on base content (`STAGE/GENERATED_AND_MOVED_FILES`).
6. Re-stage and re-verify the unit that was in review.

*Failure modes:* the stash pops onto a branch upstream changed and a hunk is silently merged into the wrong unit; an intermediate `go.sum` from the old base is staged and pins versions upstream already moved.
