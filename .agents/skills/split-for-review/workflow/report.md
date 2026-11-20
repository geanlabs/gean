<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Report

*Goal:* hand the reviewer one unit at a time, with the evidence and the reading order, and then stop. Cite this module when a unit was presented without its verification result, when the report restated the diff the user can already read, or when the skill kept going without being asked.

## REPORT/PLAN_FIRST
*Ensures:* the axis is agreed before the work.

Present the full plan once, before staging anything. Its shape follows the PR grouping (`workflow/prs.md`):

- **One PR:** a table of unit / kind / files / approximate size, in order, plus the suggested branch and its start point (`PRS/SUGGEST_BRANCH_NAMES`).
- **Several PRs:** first a table of PR / theme / suggested branch / start point / units / depends on / why it is its own PR (`PRS/SUGGEST_BRANCH_NAMES`); then the unit table for each PR; then the dependency edges (`PRS/NAMED_DEPENDENCY_EDGES`). Units merged because they could not compile apart (`ORDER/NO_DEAD_ARM`) are marked, with the reason.

Ask for a correction if the axis is a judgement call: by message vs by layer, whether to include docs, whether an unrelated fix should be separate, how many PRs.

*Failure modes:* the user wanted a different axis and says so only after all six units were built.

## REPORT/ONE_UNIT_IN_FLIGHT
*Ensures:* the reviewer is never holding two unreviewed units.

After staging one unit, stop and wait: the user may be committing, amending, rewriting the message, or rejecting the boundary. Before staging the next one, confirm the previous unit landed and the index is empty (`REPORT/STATUS`); otherwise the next unit stacks on top of it and the review boundary is gone.

*Failure modes:* three units staged into one index, so `git diff --cached` shows a pile.

## REPORT/STATUS
*Ensures:* "what is next" is answered from the repository, not from memory.

When the user asks what remains, show one table: unit / PR / state / one-line content. State is the landed commit's short sha, `staged`, or `remaining`, derived from `git log` against the plan, with the branch the next unit belongs on (`PRS/BRANCHES`). Units already landed get their sha, not their content again.

*Failure modes:* a unit reported as remaining that the user already committed under another message.

## REPORT/EVIDENCE_NOT_ASSERTION
*Ensures:* the reviewer can trust the unit boundary without re-deriving it.

Each unit's report states, briefly:
- what the unit does, and the decision the reviewer is being asked to make;
- the verification result as a fact — what was run, in the extracted tree, and the outcome (including any pre-existing failure, by name);
- anything checked that the reviewer would otherwise have to check: an identifier's full consumer list, a behaviour equivalence traced by hand, a size or layout consequence on a wire type;
- what deliberately did **not** come along, and which unit has it;
- when HEAD is the default branch, that fact, in every handover — a warning given once is forgotten by unit 3 (`PRS/BRANCHES`).

Do not paste the whole diff back; the user can read `git diff --cached`. Quote only the lines a claim depends on. Never describe a unit as reviewable while noting that it does not compile — that combination is a contradiction, not a caveat.

*Failure modes:* "this should be fine" with no run behind it; the report is a re-narration of the diff; a real consequence (a wire type growing, a dedup becoming per-root) is left for the reviewer to notice; a non-compiling unit is handed over with an apology instead of being re-cut.

## REPORT/NO_INCOMPLETE_HANDOVER
*Ensures:* a unit is never presented with a known piece missing.

If part of a unit cannot be staged cleanly — a call site sharing a hunk with a later unit's edit, say — that is a signal to restructure, not something to describe. Options, in order: author an intermediate for the file so the unit is whole; or change the code so the piece is not needed (deleting a manual call is better than splitting the hunk that contains it); or fold the piece's unit into this one.

What is never acceptable: handing over a unit while noting "its caller lands later" or "this alone does not compile". The reviewer cannot verify either half, and the author has quietly moved the cost of the bad boundary onto them.

*Failure modes:* "the call site is in unit 7, review this part now"; a unit whose `#[allow(dead_code)]` or unused-import warning is explained away in prose; the reviewer discovers the gap themselves and loses confidence in the rest of the series.

## REPORT/GROUND_EXPLANATIONS
*Ensures:* "why is this correct?" is answered from the code, not from the author's confidence.

When the reviewer questions a unit's correctness, answer with the before and after paths side by side and the exact predicate each fires on — read out of the tree, including from history (`git show <rev>:<path>`) when the old path is already committed. Name the case where the two differ, with concrete numbers.

A claim that a behaviour is preserved is worth nothing without the two predicates written down. If they are textually the same expression behind a new name, say so; if they differ, the unit is a behaviour change and must be labelled one.

Watch for the level-versus-edge trap specifically: a condition tested *after* a mutation ("is the set complete now?") is not the same as an edge ("did it just become complete?"), and swapping one for the other while claiming a pure refactor is a silent behaviour change.

*Failure modes:* "same condition, just moved" when one version is a level test and the other an edge; an explanation that asserts a consumer matters without checking whether its phase makes it inert; a reviewer left unsure and approving anyway.

## REPORT/SURFACE_WHAT_THE_SPLIT_REVEALED
*Ensures:* findings that only become visible when the change is pulled apart are not silently dropped.

Splitting exposes things a single large diff hides: an unrelated bug fix riding along, a change with no consumer yet, a scaffold that no later unit removes, a file whose edits belong to two different decisions. Report these when found, with a recommendation, and let the user decide — do not fix them inside a unit, and do not stay quiet because they are out of the split's scope.

When a unit touches a component with a design doc, check the unit against it and report the outcome: the doc confirms the shape (quote it), the unit contradicts it (the doc edit goes in the same unit), or the doc explains why a proposed "fix" is deliberate (drop the fix).

*Failure modes:* a bug fix stays buried because "the task was to split"; a speculative change with no consumer lands unremarked; a mid-series scaffold survives into the final state.
