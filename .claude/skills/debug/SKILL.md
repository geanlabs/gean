---
name: debug
description: Debug gean failures and regressions. Use when users want to (1) diagnose failing gean tests, (2) investigate devnet interoperability failures, forks, or finalization stalls, (3) trace networking, forkchoice, state transition, storage, checkpoint sync, or XMSS/FFI bugs, (4) identify which subsystem likely caused an error and what to inspect next, or (5) turn logs and test output into a concrete root-cause investigation plan.
---

# /debug - Debug gean

Use this skill as the top-level entry point for gean debugging. Start with
evidence, reduce the failing scope, then route into the right subsystem.

## Usage

- `/debug why is gean not finalizing on devnet`
- `/debug investigate this failing forkchoice test`
- `/debug trace checkpoint sync failure in gean`
- `/debug find why this XMSS test is failing`
- `/debug analyze these gean devnet logs`

## Default workflow

1. Collect the failing command, exact error text, and whether this is
   gean-only or multi-client.
2. Reproduce with the smallest failing surface before proposing fixes:
   - single package test if possible
   - existing logs before running a new devnet
   - one node or one slot range before whole-network guesses
3. Classify the failure:
   - devnet / interoperability
   - networking / gossip / peer connectivity
   - forkchoice / head selection / reorgs
   - state transition / block validation
   - storage / checkpoint sync / persistence
   - XMSS / Rust FFI / signature verification
   - test regression after a code change
4. Inspect the relevant package and only then form a root-cause hypothesis.
5. End with a short diagnosis:
   - likely subsystem
   - evidence observed
   - next verification command or file to inspect

## Routing and stop conditions

- If devnet logs already exist, start with
  `./.claude/skills/devnet-log-review/scripts/analyze-logs.sh` and use the
  `devnet-log-review` skill for log-specific investigation.
- If the issue needs a fresh multi-client reproduction, use the
  `devnet-runner` skill.
- If the task is validating a branch/PR against peer clients, use the
  `test-pr-devnet` skill.
- Do not re-run a whole devnet when a targeted `go test` reproduces the issue.
- If the user has not provided logs, error text, a failing command, or a clear
  reproduction path, stop and ask for the missing evidence before guessing.
- If no minimal reproduction can be identified, say that clearly and return the
  best next step for obtaining one.

## Investigation entry points

### Devnet / interoperability

Use this when gean disagrees with peers, stalls, or produces invalid blocks.

```bash
# Analyze existing logs in repo root or a specified directory
./.claude/skills/devnet-log-review/scripts/analyze-logs.sh

# Show detailed gean errors from the current log set
./.claude/skills/devnet-log-review/scripts/show-errors.sh -n gean_0 -w
```

Focus on the first divergent slot or first rejected block, not the tail of the
failure. Compare gean's view of head, justified, and finalized slots against
peer clients before changing code.

### gean unit or package regressions

Use this when the failure is local to one package or appears in CI/unit tests.

```bash
# Fast default suite: excludes XMSS FFI and spec fixtures
make test

# Package-focused run
go test ./forkchoice -run TestName -v -count=1
go test ./node -run TestName -v -count=1
go test ./statetransition -run TestName -v -count=1
```

Prefer the narrowest failing package and search for the exact error string
before opening unrelated code:

```bash
rg -n "error text here|panic text here" .
```

### Spec fixture failures

Use this when gean diverges from the pinned leanSpec behavior.

```bash
make test-spec
go test ./spectests/ -count=1 -tags=spectests -run TestName -v
```

Treat spec fixture failures as deterministic until proven otherwise. Inspect
`spectests/`, `statetransition/`, `forkchoice/`, and `types/` before assuming
networking problems.

### XMSS / Rust FFI / signature issues

Use this when signatures fail, FFI panics, or aggregate proofs look wrong.

```bash
make test-ffi
go test ./xmss -run TestName -v -count=1
```

Inspect `xmss/ffi.go`, key/proof handling, and `xmss/rust/` before making
changes elsewhere. Verify whether the failure is in Go-side marshaling, Rust
glue, or test fixtures.

### Checkpoint sync / storage / persistence

Use this when nodes fail to resume, finalized state looks wrong, or sync from a
checkpoint diverges.

```bash
# Inspect the finalized state endpoint from a running source node
curl http://127.0.0.1:5052/lean/v0/states/finalized -o /tmp/finalized.ssz

# Read the justified checkpoint JSON
curl http://127.0.0.1:5052/lean/v0/checkpoints/justified
```

Confirm the source node is healthy before blaming the syncing node. Only clear
data directories or restart from genesis when the user wants a fresh-state
reproduction.

## Gean-specific heuristics

- Forks usually become visible first as a slot-level disagreement, not a final
  error line.
- Finalization stalls are usually easier to debug from participation/head
  progress than from the eventual timeout symptom.
- If a block is reproducibly invalid from the same inputs, investigate
  `statetransition/` or `types/` before `p2p/`.
- If gean receives and publishes but does not advance head, inspect
  `forkchoice/` and `node/store_block.go`.
- If peers do not connect or topics are silent, inspect `p2p/` and node startup
  flags before touching consensus logic.

## References

- See [references/debug-paths.md](references/debug-paths.md) for symptom to
  subsystem mapping.
- See [references/debug-workflows.md](references/debug-workflows.md) for the
  detailed triage flows.

## Response format

Always end with this structure:

```markdown
## Debug Summary

**Scope:** {gean-only | multi-client}
**Failing surface:** {test | spec fixture | xmss/ffi | devnet | checkpoint sync | unknown}
**Likely subsystem:** {subsystem}
**Evidence:** {1-3 concrete observations}
**Next step:** {single best verification command or file to inspect}
```

Do not end with a generic conclusion. If the root cause is still unproven, say
that explicitly and use `Next step` for the best verification action.
