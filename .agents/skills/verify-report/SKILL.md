---
name: verify-report
description: Re-verify an earlier answer, review or recommendation claim by claim against the source, then correct it. Use when asked to "verify your report", "recheck your answer", "double-check your answer" or "re-review your review".
argument-hint: "[what to verify — defaults to the last substantive answer]"
---

# Verify report

You wrote the answer under review and are anchored on it. Assume every claim is wrong until fresh
evidence shows it holds; if it holds, say so and move on. Do not apologise.

Target: $ARGUMENTS (if empty, the most recent substantive answer, review or recommendation).

## List the claims

For yourself, list every checkable statement in the target, each short enough to be true or false:

- **Facts about the code or files:** "X calls Y", "N places do Z", "package P handles slots".
- **Comparisons:** "new", "stricter", "missing", "unlike project Q", "already covered".
- **Behaviour of tools, products or APIs:** "the harness loads symlinks", "the linter catches this".
- **Recommendations:** each "you should…" or "consider…".

Skip opinion and wording.

## Verify each claim, fresh

Earlier tool output in the conversation is not evidence on its own: the old answer may have misread
it. Group claims by source, read each file or diff once, and run independent checks in parallel.
Claims about the conversation itself need no tool call.

Common failure modes:

1. **Counting grep hits as matches.** A search found N lines, so the answer said N things are
   wrong. Open each hit and read its surrounding code: some will be legitimate exceptions.
2. **Stating from inference or memory.** A package's purpose guessed from its name, or a tool's
   behaviour recalled rather than checked. Read the file, docs, `--help` or source.
3. **"New" or "worse" without a baseline.** A claim that a change made something stricter, added a
   problem, or is missing something. Check the previous version (`git diff`, `git show HEAD:path`)
   or the other place it may already exist.
4. **Importing another project's convention.** A suggestion copied from a reference project.
   Check the target project actually works that way (branch names, layout, tooling).
5. **Already covered, or contradicted, by the project.** A recommendation that `.golangci.yml`, the
   Makefile, `.github/workflows/`, `AGENTS.md` or an `.agents/rules/` file already enforces or
   rules out. Grep those for it.
6. **Severity and scope.** A minor issue presented as major, a three-line fix presented as a
   pattern, or a guess presented as certain.
7. **Omissions.** Something the user asked that the answer skipped, or a problem found while
   checking the claims above.

If a claim can't be checked, mark it unverifiable rather than guessing either way.

## Report

Lead with the bottom line in one or two sentences: does the original answer stand, mostly stand, or
need replacing? If everything held, that line is the whole report.

Then a table of every claim that changed or is in doubt, and one line beneath it summarising the
claims that held:

| Claim (as originally stated) | Verdict | Evidence |
|---|---|---|

Verdicts: **Wrong**, **Overstated**, **Understated**, **Unverifiable**. Evidence is a `file:line`,
command output or doc URL.

Then the correction. If the answer mostly stands, give only what changes. If it needs replacing,
give the corrected answer in full. Drop points that no longer hold rather than hedging them.
