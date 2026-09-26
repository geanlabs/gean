---
description: gean protocol-engineering directive — apply before any fix, feature, refactor, optimization, or test. Enforces spec-first development and a required post-implementation review.
argument-hint: [task to implement]
---

You are working inside **gean**, a Go Ethereum **lean consensus** client tracking the leanSpec pinned at
`LEAN_SPEC_COMMIT_HASH` in the `Makefile`. `AGENTS.md` is the canonical guide: follow its Principles,
Notes, and Code Style. This command adds only the working order below.

The task to implement:

$ARGUMENTS

## Priorities
Optimize in this order: **spec compliance → consensus correctness → safety → clarity → performance →
operations.** When goals conflict, the higher one wins.

## Before writing code
- Read the relevant pinned spec section and state which spec function the change mirrors, why it complies,
  and its edge cases.
- When gean and the spec disagree, fix gean. When the spec is ambiguous, stop and surface the ambiguity to
  the user; do not copy another client's behavior.
- Ask: can this be solved by deleting code? Can an existing gean package be reused? Is this a protocol
  concern or incidental? Does the added complexity earn its keep?

## Attribution
Commit as the user running the session. Never add Claude/Anthropic/AI references, `Co-Authored-By: Claude`,
or "Generated with Claude Code" to code, commits, PRs, or issues.

## Required review before declaring done
Run `/lean-review` for simplification and `/spec-compliant` when consensus behavior changed. For live
behavior, use the devnet skills on the user's devnet server. Then report:
- **Correctness**: edge cases and invariants (fork-choice weights, vote-index remap on prune,
  `state_root` verification).
- **Simplicity**: what could still be removed.
- **Spec compliance**: matches the pinned spec; assumptions justified.
- **Architecture**: ownership of shared state and background work; FFI and sszgen rules honored.
- **Testing**: behavior covered; `make test-spec` green for consensus changes.
- **Checks run**: what ran, results, and what was skipped.
