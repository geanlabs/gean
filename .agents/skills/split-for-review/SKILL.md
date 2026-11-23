---
name: split-for-review
description: Split an uncommitted change into a sequence of independently reviewable, independently compiling units, staged one at a time for the user to review and commit. Use when a working tree has grown too large to review in one pass, or when the user asks to split changes by parts, stage them one by one, or make a reviewable commit series.
---

# Split for review

Turn one large uncommitted change into an ordered series of units. Each unit is staged alone,
verified to build and pass by itself, and handed to the user to review and commit.

The skill never commits, pushes or rewrites existing commits, and never fixes or refactors on its
own: the units add up to exactly the original change. It writes only to the index. The worktree
changes in two cases, each only when the user asks: a design change requested mid-review (apply
it to the worktree, the snapshot and every intermediate still to be staged), and moving the
series after a lower PR merges (step 10).

`split.sh` does the git plumbing; run it with no arguments for its usage:

```
split.sh baseline              # authoritative run on the whole change, before splitting
split.sh snapshot              # copy the change to the snapshot dir, empty the index
split.sh stage <path> [<from>] # stage the snapshot's (or <from>'s) content as <path>
split.sh rm <path>             # stage the removal of <path>
split.sh unstage <path>        # drop <path> from the index
split.sh lock                  # resolve go.mod and go.sum for the index and stage them
split.sh verify                # authoritative: gofmt, make lint, make test on the extracted index
split.sh check <packages...>   # fast, scoped go vet and go test for iterating — not a verdict
split.sh union-check           # do the units so far add up to the original change?
split.sh drift                 # has the base moved, other than by landed units?
split.sh stale                 # is the branch's PR merged, or main ahead? which files overlap?
split.sh base-version <path>   # write HEAD's version of <path>, to author an intermediate from
```

## Procedure

1. **Pin the base.** Record and state the branch and `git rev-parse HEAD`. Fetch and run
   `split.sh stale`: if the branch's PR already merged or the default branch moved, propose moving
   onto `origin/<default>` (step 10) before cutting anything. Before each later unit, run
   `split.sh drift`; a move other than a landed unit means edits may be lost, so stop, name the
   files that now differ from the snapshot, and re-snapshot before going on.
2. **Prove the whole change green.** Run `split.sh baseline` and record each check's result, the
   count of passed tests it prints, and every pre-existing failure by name.
   Later failures are judged against it.
3. **Snapshot.** With an empty index, `split.sh snapshot` (`SPLIT_EXCLUDE` keeps unrelated
   untracked paths out). The snapshot is the authority for what every file looks like at the end.
4. **Derive units by change, not by file.** One file can carry several units' edits; one change
   (a type plus every consumer it forces) is one unit, however many files it spans. For each
   changed identifier, grep the whole repo, build-tagged files included, for everything that
   breaks without it. A unit is what a reviewer would accept or reject for one reason, and shows
   the whole mechanism: who produces a fact, who acts on it, and what old path went. Keep a
   unit's production diff under about 250 lines, measured apart from its tests. Units fall into
   recurring kinds: a parameter bundle, plumbing, a behaviour-preserving data structure, and the
   behaviour change itself.
5. **Order them.** Sort on dependencies, then by kind: independent pieces (a fix found in passing,
   a bump, a moved helper) first, contracts with their consumers, mechanical follow-through,
   behaviour in data-flow order, then harness and docs. Mutually dependent units are one unit;
   never break a cycle with a shim no later unit removes. Every intermediate must behave, not just
   build: change a fact's producer and consumer in the same unit. An old route may live one extra
   unit, deleted by the unit that switches the producer; a new route must not land before the
   code that reaches it.
6. **Plan PRs and branches.** Default to one PR; split only when the user asks or themes have
   different reviewers or landing times, never one PR per unit. For each later PR, name the exact
   symbol it needs from an earlier one, or base it on the default branch. Propose each branch:
   the prefix and kebab-case style of recent branches (`git branch --sort=-committerdate`), named
   for the change, checked free locally and on origin, with
   `git checkout --no-track -b <name> <start>`. If HEAD is the default branch, say so in every
   handover. Show the whole plan (units, kinds, files, sizes, branches) before staging anything,
   and drop any kind the user doesn't want.
7. **Stage one unit in the index.** A file wholly in the unit: `split.sh stage <path>`. A file
   shared with later units: start from `split.sh base-version <path>`, apply only this unit's
   edits as scripted exact replacements (never `git add -p` or a patch made up front), stage it,
   and check that what remains against the snapshot is exactly the later units' edits. Lock files
   come from `split.sh lock`; a rename is `stage` plus `rm`.
8. **Verify it alone.** While reshaping, use `split.sh check ./<pkg>/...`. Before handing it over,
   `split.sh verify`, judged by its exit status. A failure not in the baseline belongs to the unit;
   a test count well below the baseline means a suite skipped. A definition whose first use is in
   a later unit fails `make lint`'s `unused` check, and an import left by a partial edit doesn't
   compile: both mean the boundary is wrong, so move the use or the definition.
9. **Hand over one unit, then stop.** Say what it does and the decision it asks for, what was run
   and its result (pre-existing failures by name), what you checked that the reviewer would
   otherwise have to, and what was left for which later unit. Never hand over a unit with a piece
   missing; re-cut it. Report what the split revealed (a buried fix, a change with no consumer)
   for the user to decide. Before the next unit, confirm the last one landed and the index is
   empty. After the last, `split.sh union-check` must show the index equal to the snapshot and
   nothing else changed.
10. **After a lower PR merges** (only when the user asks): fetch; list the changed files the
    merged PR also touched (`split.sh stale`); `git stash push -u`, check out the target,
    `git merge --ff-only`, `git stash pop`, and stop on any conflict. Diff every changed file
    against the old snapshot: only those files may differ, and only by the merged PR's hunks. Then
    take a new snapshot and baseline, regenerate intermediates and lock files, and re-verify the
    unit in review.

When asked what remains, answer from `git log` against the plan: one row per unit with its
landed sha, `staged` or `remaining`. When asked why a unit is correct, put the old and new
predicates side by side from the tree; a check on a state swapped for a check on a transition is
a behaviour change.
