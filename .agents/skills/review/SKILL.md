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
   changes, read the leanSpec function the code implements (pinned commit:
   `LEAN_SPEC_COMMIT_HASH` in the Makefile).

## Phase 2: Review, one file at a time

Skip what `make lint` already enforces: formatting, vet findings, and the import rules.

### Consensus correctness (critical)

- Does the code do what leanSpec does, including edge cases: slot 0, empty attestations, the
  finalized boundary, duplicate or out-of-order messages?
- State transition and fork choice must be deterministic: no map iteration order, wall-clock time
  or goroutine timing deciding a result.
- Validity checks: is anything accepted that the spec rejects, or rejected that it accepts?
- SSZ: struct tags and field order match the spec; generated `*_encoding.go` files were
  regenerated (`make lean-sszgen`), not edited.

### Concurrency and lifecycle

- The engine is a single-writer dispatch loop. Is state it owns read or written from another
  goroutine (gossip validators, req/resp handlers, the sync driver, workers)?
- Every goroutine has a way to stop, and something waits for it: started with `lifecycle.Group`
  (`tasks.Go`), stopped by the context, joined before the store or keys it uses are closed.
- Channels: who closes them, can a send block forever, is a full channel dropped on purpose?
- Locks: held across blocking calls or I/O? Consistent order wherever two are taken?

### Resources

- XMSS FFI: every `ParsePublicKey`/`ParseSignature` has a matching `Free`, or is owned by a cache
  that frees it; keys are closed only after every proof using them has finished.
- Timers and tickers stopped; Pebble iterators and batches closed; HTTP response bodies closed.
- Maps, buffers and caches bounded, and pruned at finality where they track chain data.

### Errors and inputs

- Peer input (gossip, req/resp, the API) is validated before use, with size limits before
  allocation; a malformed message is rejected, never a panic.
- Errors are returned or logged with context, not dropped; a peer's fault is not logged as ours.

### Design and project rules

- `internal/core` imports no chain; shared code that knows about lean belongs in `internal/lean`.
- No backward-compatibility shims, aliases or fallbacks.
- Comments explain why; no references to plans, phases or the session.
- The diff stays on its topic: no unrelated renames or rewrites.
- Could the same result come from less code: removing something, or reusing what exists, instead
  of adding? Prefer the smaller change, and suggest it.

### Tests

- A bug fix has a regression test that fails without the fix.
- New behaviour is tested where it lives, written like the tests around it.
- Tests wait on conditions rather than sleeping, and pass with `-count=1` repeatedly.
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

Leave out empty sections. On GitHub, post the review as a comment, never "request changes", and say
it was written by an AI agent, naming the harness and model.
