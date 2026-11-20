---
name: split-for-review
description: Split an uncommitted change into a sequence of independently reviewable, independently compiling units, staged one at a time for the user to review and commit. Use when a working tree has grown too large to review in one pass, or when the user asks to split changes by parts, stage them one by one, or make a reviewable commit series.
---

<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Split for review

## What this skill does
Turn one large uncommitted change into an ordered sequence of units. Each unit is staged into the index alone, verified to build and test green **by itself**, and presented for the user to review and commit. The skill produces the split and the evidence; the user commits.

## What this skill does not do
- Commit, push, or create branches. Staging is the skill's last action per unit.
- Modify the worktree, except in the two cases `STAGE/INDEX_ONLY` names, each only when the user asks.
- Change behaviour, fix bugs, or refactor on its own initiative. The union of all units equals the original change (`STAGE/UNION_IS_THE_DIFF`); a design change the user asks for mid-review is applied everywhere (`STAGE/PROPAGATE_DESIGN_CHANGES`).
- Reorder or rewrite existing commits. Only uncommitted work is in scope.

## Invariants
1. A unit that does not compile is not a unit (`workflow/verify.md`).
2. Split by change, not by file (`UNITS/CHANGE_NOT_FILE`).
3. The worktree is read-only (`STAGE/INDEX_ONLY`).
4. Every intermediate state behaves, not just builds (`ORDER/NO_BROKEN_MIDDLE`).
5. One unit = one reason to reject (`UNITS/GROUP_BY_COMPLETE_PICTURE`).
6. The base is pinned and re-checked (`BASE/PIN_AND_DETECT_DRIFT`).
7. Independent pieces land first (`ORDER/COMPILE_ORDER_FROM_DEPENDENCIES`).

## Workflow
Each step is owned by exactly one module. Procedural detail does not live in this file.

| step | module |
|---|---|
| 1. Pin the base, move it onto the latest default branch if stale, prove the whole diff is green, snapshot it | `workflow/base.md` |
| 2. Derive units from the diff and their dependency graph | `workflow/units.md` |
| 3. Order the units so no intermediate is broken | `workflow/order.md` |
| 4. Group the units into PRs, branches, and move the series when a PR merges | `workflow/prs.md` |
| 5. Assemble each unit in the index | `workflow/stage.md` |
| 6. Verify the unit standing alone | `workflow/verify.md` |
| 7. Present the plan, one unit at a time, and status on request | `workflow/report.md` |

`.agents/skills/split-for-review/split.sh` implements the mechanical parts. Run it with no arguments for its usage; use it rather than re-deriving the git plumbing:

```
split.sh baseline              # authoritative run on the whole change, before splitting
split.sh snapshot              # copy the change to the snapshot dir, empty the index
split.sh stage <path> [<from>] # stage the snapshot's (or <from>'s) content as <path>
split.sh rm <path>             # stage the removal of <path>
split.sh unstage <path>        # drop <path> from the index
split.sh lock                  # resolve go.mod and go.sum for the index and stage them
split.sh verify                # authoritative: make lint and make test on the extracted index
split.sh check <packages...>    # fast, scoped go vet and go test for iterating — not a verdict
split.sh union-check           # do the units so far add up to the original change?
split.sh drift                 # has the base moved, other than by landed units?
split.sh stale                 # is the branch's PR merged, or main ahead? which changed files did upstream touch?
split.sh base-version <path>   # write HEAD's version of <path>, to author an intermediate from
```
