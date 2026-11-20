<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Stage

*Goal:* put exactly one unit in the index without touching the worktree. Cite this module when the worktree was modified or reverted during a split, when a unit's content drifted from what the user wrote, or when hunk-picking turned into a fight with the tool.

## STAGE/INDEX_ONLY
*Ensures:* the user's final state survives the whole split, and nothing needs undoing if the split is abandoned.

Assemble units in the index. Never revert, stash, or check out over the worktree to produce an intermediate: the worktree holds work that may exist nowhere else (untracked files have no reflog), and a destructive command is both unnecessary here and likely to be refused.

The mechanism is content-addressed staging, which needs no worktree write:

```
sha=$(git hash-object -w <content-file>)
git update-index --add --cacheinfo 100644,$sha,<repo-relative-path>
```

`split.sh stage <path> [<content-file>]` wraps this, defaulting the content to the snapshot version. To take a path back out of the index, `git reset -q -- <path>` (index-only) or `split.sh unstage <path>`.

Everything the split needs is available without a single worktree mutation: `git show HEAD:<path>` reads the base version, the snapshot holds the final version, and authored intermediates live in the scratch directory.

There are exactly two exceptions, each only when the user asks: applying a design change the user requested mid-review (`STAGE/PROPAGATE_DESIGN_CHANGES`), and moving the series after a lower PR merged (`PRS/AFTER_LOWER_PR_MERGES`).

*Failure modes:* `git checkout -- .` discards the entire change; `git stash` hides untracked files and they are forgotten; a tracked file is edited to produce an intermediate and the user's version is never restored; the split is abandoned halfway and the worktree is left in an invented state.

## STAGE/AUTHOR_SPLIT_FILES
*Ensures:* a file shared between units carries exactly its own unit's edits, with no leakage from the others.

For a file owned entirely by one unit, stage the snapshot version. For a file split across units, author the intermediate: start from the version already in HEAD (`git show HEAD:<path>`) and apply only this unit's edits, deterministically — a scripted set of exact string replacements, not hand editing, and not `git add -p` guessing at hunk boundaries.

Prefer scripted replacement over patch application: hunk offsets shift as earlier units land, so a patch generated up front against the original base will fail or, worse, apply in the wrong place. Regenerate content, not diffs.

Two checks after authoring:
- Diff the staged version against the snapshot (`git diff --cached -- <path>` versus the snapshot content) and confirm the remaining difference is exactly the *later* units' edits, nothing more.
- Re-read the authored region. A scripted replacement across a whole file produces artefacts a compiler and even a linter may accept — a doubled blank line, a comment left describing the old code, an import used only under one build tag. Run the repo's formatter check on the extracted tree (`workflow/verify.md`) rather than trusting the edit.

*Failure modes:* a mechanical replace leaves a stale comment in the staged version while the user's final version rewords it, so the last unit carries a cosmetic diff nobody asked for; a patch generated against the original base silently applies at the wrong offset; hunk-picking with `git add -p` splits a hunk that contained two units' edits and the wrong half lands.

## STAGE/FORMAT_EVERY_TREE
*Ensures:* the formatter check passes on the *index*, not just on the worktree.

The repo's formatter fixes the worktree, never an intermediate, and the intermediate is what gets staged. The extract's format check is the judge (`VERIFY/EXTRACT_THE_INDEX_TREE`): when it fails, apply its diff to the intermediate, re-stage, and re-verify.

*Failure modes:* `split.sh verify` reports unformatted files on the extracted index while the worktree is clean, because a hand-wrapped line in the intermediate was never formatted.

## STAGE/PROPAGATE_DESIGN_CHANGES
*Ensures:* mid-review design feedback lands everywhere, not just in the unit being discussed.

When the user asks for a change to a unit's *design* (a rename, an inlined wrapper, a revert), the skill applies it to all three copies: the worktree, the snapshot, and every authored intermediate still to be staged. Otherwise the later units are built on a shape that no longer exists, and the union no longer equals the change.

Before building the next unit: `split.sh drift` shows the worktree equal to the snapshot, and the whole worktree still passes the repo's check.

*Failure modes:* a unit is reworked into a better shape while the worktree still holds the old one, and the later units are authored against the dead design; the final unit carries a surprise revert.

## STAGE/SHELL_TRAPS
*Ensures:* the split's plumbing cannot silently do nothing, hang, or kill its own shell.

- `cp`, `mv` and `rm` are commonly aliased to their `-i` forms. With no stdin the prompt is answered "no" and the copy is **skipped while the command still reports success**; in a backgrounded command it waits forever. Use `command cp -f`, or a language runtime (`python3 -c 'shutil.copyfile(…)'`), and verify by comparing bytes.
- `pkill -f <pattern>` kills the shell running it whenever the pattern also matches its own command line.

*Failure modes:* a snapshot silently not updated, so the next unit is authored from a stale base.

## STAGE/GENERATED_AND_MOVED_FILES
*Ensures:* files that are not hand-edited still land in the right unit, exactly.

- **Lock files** are resolved, never authored: `split.sh lock`. Its diff against the snapshot's lock must be exactly the later units' lines.
- **Renames and deletions** stage both sides: `split.sh stage` for the new path, `split.sh rm` for the old one.

*Failure modes:* a hand-edited lock pins a version the unit's manifests never asked for; a rename staged as an add only, so the old file survives in the commit.

## STAGE/UNION_IS_THE_DIFF
*Ensures:* splitting is lossless — the series ends exactly where the worktree started.

After the last unit is staged, the index must equal the snapshot for every file, and `git diff` (worktree vs index) must be empty. Check this explicitly before declaring the split done; if it is not empty, the remainder is a unit that was forgotten, not a rounding error.

Also confirm nothing was gained: no file appears in the union that was not in the original change, and no reformatting rode along. A formatter run during the split can touch files no unit claims.

*Failure modes:* a file is left half-staged and the last commit carries an unexplained fragment; a formatter run mid-split reformats an untouched file and it lands in whichever unit was staged next; the split "completes" while the worktree still differs from HEAD.
