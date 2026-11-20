<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Units

*Goal:* carve the diff into units along the axis a reviewer actually judges — one decision per unit — while keeping each one compilable. Cite this module when a unit could not be reviewed without reading another one, when a "unit" was really a pile of unrelated edits that happened to share a directory, or when a unit failed to build because its dependents were left out.

## UNITS/CHANGE_NOT_FILE
*Ensures:* units follow logical changes, not path prefixes.

Grouping by file or package is the default mistake, and it fails in both directions:

- **One file carrying several changes** — a shared message/type/schema module is the usual case. A wire-type file that gained three unrelated fields belongs to three units, one hunk group each. Splitting it needs an authored intermediate per unit (`workflow/stage.md`), not a hunk-by-hunk `git add -p` guess.
- **One change spanning many files** — a type change plus every consumer that had to follow. Those files go in *one* unit. Staging the type alone produces a tree that does not compile, which is not a reviewable unit no matter how coherent the diff reads.

The test is not "do these lines live together" but "would a reviewer approve or reject these lines for the same reason, and can they see the consequence". A definitional change is never smuggled into a behaviour unit because it was "one line" — the one line is often the actual decision.

*Failure modes:* a `messages.go`-only unit is staged and declared reviewable while every consumer of the changed field is left in a later unit, so nothing compiles; twelve mechanical call-site updates are spread across four units because they lived in four packages; a unit is described to the user as "does not compile on its own, that's expected" — it never is.

## UNITS/DEPENDENCY_DIRECTION
*Ensures:* each unit carries everything that must change with it, derived from the direction the dependency actually points.

For each candidate change, walk both directions before fixing its boundary:

- **Down** — what does this change require to already exist? A new enum variant needs its type; a call needs its signature.
- **Up** — who breaks if this lands alone? Grep the changed identifier across the whole module, including test-only and build-tag-gated code: a build-tag-gated test binary is invisible to a check that does not enable its feature. Every hit is either in this unit or proof that the boundary is wrong.

A renamed or retyped field, a changed function signature, a removed variant, and a changed re-export all have unbounded blast radius until the grep says otherwise. Do the grep; do not reason about who "probably" uses it.

*Failure modes:* a message field is retyped and the harness binary that constructs it is missed because it only builds under a build tag; a deleted enum variant leaves a match arm in a package nobody thought to check; a unit is sized by reading the diff instead of by grepping the identifier, and the tenth consumer surfaces during verification.

## UNITS/SIZE_CEILING
*Ensures:* a unit is small enough to review in one sitting, even when it already compiles and is already "one idea".

Compiling is necessary, not sufficient. A unit whose **production** diff exceeds roughly 250 lines is too big regardless of how coherent it feels: at that size the reviewer stops reading and starts skimming, which is the failure the split existed to prevent.

Measure before presenting, and measure production and test lines *separately* — a test-harness rewrite can be the majority of a diff and is reviewed differently from logic:

```
git diff --cached -w --stat                      # indentation-only churn removed
awk over `git diff --cached -U0` split at the `mod tests` hunk boundary
```

When a unit is over the ceiling, look for these seams, which recur: a constructor/parameter bundling; a plumbing change (how handlers reach their output); a data-structure introduction that is behaviour-preserving; and the behaviour change itself. Those are four units, and the behaviour change — the only one that needs real scrutiny — is usually the smallest.

*Failure modes:* a 578-line "one idea" unit that was four ideas; a reviewer asked to audit a 150-line test-rig rewrite and a semantic change in the same pass; the size only becomes apparent when the diff is already staged and the author is invested in it.

## UNITS/GROUP_BY_COMPLETE_PICTURE
*Ensures:* each unit shows one whole mechanism, so the reviewer never has to hold two units in their head.

Group by the mechanism a reviewer must understand, not by where a symbol happens to live. The commonest mistake: putting a consumer's *routing* change in the same unit as the *field* it reads, because both touch the wire type — while the producer that makes the routing reachable lands two units later. The routing belongs with the producer.

The test: for each unit, can the reviewer see who emits the fact, who acts on it, and what old path was removed? If any of the three is elsewhere, the boundary is wrong.

A widened contract with its producers labelling themselves is a complete picture. A contract plus half its consumers is not.

*Failure modes:* a routing arm reviewed with no producer in sight, so its correctness cannot be judged; the author explains the mechanism in prose across three unit descriptions instead of showing it in one diff; a reviewer asks "why is this correct?" and the honest answer requires a later unit.

## UNITS/TAXONOMY
*Ensures:* the split has a shape the reviewer can predict, and cheap units are not buried inside expensive ones.

Sort the diff into these kinds; their order is `ORDER/COMPILE_ORDER_FROM_DEPENDENCIES`'s.

1. **Independent piece** — anything the feature does not need, or needs only as a finished fact: a bug fixed in passing, a schema cleanup, a client or transport swap, a version bump, a helper moved into a shared package. Own unit, so it can be cherry-picked, back-ported, or landed while the rest is still under review.
2. **Contract change** — a type, message, schema, or signature, *with* every consumer it forces. Behaviour-preserving where possible.
3. **Mechanical follow-through** — a rename or re-typing rippling through call sites with no decision in it. Boring by design; keep it boring and keep it separate, so the reviewer can skim it instead of auditing it inside a unit that has real content.
4. **Behaviour change** — the actual feature. Cut it where its data changes hands — generated at build time → set up at start → produced at run time → consumed — and ask the user for the stage names rather than inventing them.
5. **Test-only / harness** — a measurement rig or scenario whose semantics changed. Own unit: a harness that silently redefines what it measures is its own review.
6. **Docs / prose** — no code. Own unit, orderable anywhere; skip it if the user says so.

Ask which categories are wanted before building anything: "ignore docs, start from 2" is a normal answer, and a unit built for a category the user did not want is wasted work.

The classification is shown to the user before anything is staged (`REPORT/PLAN_FIRST`).

*Failure modes:* an unrelated bug fix is buried in a 500-line feature unit and cannot be landed separately when the feature is held up; a harness change that alters what a benchmark counts is reviewed as "just tests"; the feature is split by file, so the reviewer reads the consumer before its producer exists.
