---
name: review
description: Deep code review of a gean change or pull request - consensus correctness against leanSpec, concurrency and lifecycle, resource leaks, tests and project rules. Use when asked to "review", "check this PR", or to look over a diff before it merges.
---

# Code review

Review for correctness first, then for what would cost a reviewer or operator time later. Report
only what you can point to in the code; a finding you can't tie to a line is not a finding.

## Phase 1: Gather context

1. Get the change: `git diff <base>...HEAD`, or `gh pr diff <number>` and `gh pr view <number>`.
2. Read the description and any linked issue: what is the change for?
3. List the touched packages and read each changed file in full, not just the hunks. For consensus
   changes, read the leanSpec function the code implements, at the commit named beside
   `LEAN_SPEC_FIXTURES_SHA256` in the Makefile.

## Phase 2: Review, one file at a time

Skip what prep-commit already enforces: formatting and anything `make lint` reports.

Check each file against the `AGENTS.md` rules and the `.agents/rules/` file for each area the diff
touches, then ask:

- Where two locks are taken, is the order the same everywhere?
- Are timers and tickers stopped?
- Are errors returned or logged with context, not dropped?
- Is new behaviour tested where it lives?
- Consensus changes run `make lean-spectest`; node, sync and p2p changes run `make lean-e2e`.

## Report

Order findings by severity. Each one names the file and line, what is wrong, why it matters
(a failure it causes), and the fix.

```
## Review: <title>

### Critical
- `internal/lean/forkchoice/votes.go:88` — <what is wrong>. <the failure it causes>. Fix: <fix>.

### Major
### Minor
### Nits

### Verdict
<one or two sentences: safe to merge, or what must change first>
```

Leave out empty sections. On GitHub, post the review as a comment, never "request changes".
