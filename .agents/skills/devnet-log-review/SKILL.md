---
name: devnet-log-review
description: Review and analyze lean devnet run results. Use when asked to (1) analyze devnet logs for errors and warnings, (2) summarize a devnet run, (3) find interoperability issues between gean and other clients, (4) check block production and consensus progress, or (5) debug forks and stalled finalization.
---

<!-- Adapted from ethlambda's devnet-log-review skill (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Devnet log review

Analyze and summarize a lean devnet run, typically one started with `make lean-devnet` or the
[`devnet-runner`](../devnet-runner/SKILL.md) skill.

## Quick start

```bash
# Logs in the current directory
.agents/skills/devnet-log-review/scripts/analyze-logs.sh

# Or a directory of logs
.agents/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/logs
```

It reports errors and warnings per node, blocks proposed and imported, the last slot each node
reached, and which slots each node proposed.

## Log files

| File | Content |
|---|---|
| `devnet.log` | Combined output of lean-quickstart's `spin-node.sh` (genesis and all nodes) |
| `{client}_{n}.log` | One node's log, e.g. `gean_0.log`, `zeam_0.log`, `ethlambda_0.log` |

## Scripts

| Script | What it does |
|---|---|
| `analyze-logs.sh [dir]` | Runs everything below and prints a markdown summary |
| `count-errors-warnings.sh [dir]` | Errors and warnings per node, without known-benign messages |
| `count-blocks.sh [dir]` | Blocks proposed and imported per node, per client's log format |
| `check-consensus-progress.sh [dir]` | Last slot per node, and the slots each node proposed |
| `show-errors.sh [-n node] [-l limit] [-w] [dir]` | The error (and with `-w`, warning) lines themselves |

## gean's log lines

gean logs `timestamp LEVEL [component] message`. The lines that matter most:

```
INFO [validator] proposed block slot=X block_root=0x... attestations=N     # a block this node built
INFO [chain] block slot=X ... justified_slot=J finalized_slot=F             # a block this node imported
INFO [forkchoice] head slot=X head_root=0x...                                # head moved
INFO [forkchoice] finalized advanced slot=X root=0x...                       # finality moved
INFO [signature] aggregate: slot=X raw=N ...                                 # aggregator built a proof
  Latest Justified:   Slot J | Root: 0x...                                   # in the periodic CHAIN STATUS block
  Latest Finalized:   Slot F | Root: 0x...
```

A healthy proposal: `proposing block slot=X validator=Y`, then `proposed block slot=X ...`.
`discarding proposal slot=X: head moved` means a block for that slot arrived first; occasional
ones are normal. Other clients' formats are in
[references/CLIENT_LOG_PATTERNS.md](references/CLIENT_LOG_PATTERNS.md).

## Common investigations

Strip ANSI colour codes first: `sed 's/\x1b\[[0-9;]*m//g' file.log`.

```bash
# Everything about one slot, across nodes
grep -hE "slot=12([^0-9]|$)" *.log

# Block roots at a slot: more than one means a fork
grep -h "slot=4[^0-9]" *.log | grep -oE "block_root=0x[a-f0-9]{16}" | sort -u

# Finality over time on a gean node
grep "Latest Finalized" gean_0.log | tail -20

# Rejected blocks
grep "block processing failed" gean_*.log
```

A validator proposes when `slot % validator_count == validator_index`.

**Justification needs a two-thirds supermajority**, `3 * votes >= 2 * validator_count`
(`internal/lean/transition/attestations.go`): 6 validators need 4 votes, 9 need 6, 16 need 11.
At least one node must aggregate (`--is-aggregator`), or blocks carry no attestations and
nothing is ever justified.

Deeper guides, loaded only when needed:

- [references/FORK_ANALYSIS.md](references/FORK_ANALYSIS.md): fork types, tracing parents,
  drawing the fork structure, which validators are on which fork.
- [references/FINALIZATION_DEBUG.md](references/FINALIZATION_DEBUG.md): causes of finality
  stalls, participation math, the justification chain.
- [references/ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md): critical, medium and
  benign errors.
- [references/CLIENT_LOG_PATTERNS.md](references/CLIENT_LOG_PATTERNS.md): log formats of gean,
  zeam, ream, ethlambda, grandine, lantern and qlean.

## Summary format

Keep it under 20 lines:

```markdown
## Devnet log summary

**Run:** {N} nodes ({clients}) | {M} slots ({range})

| Node | Validators | Blocks proposed | Errors | Warnings | Status |
|---|---|---|---|---|---|
| gean_0 | 0, 3 | 4 (slots 3, 6, 9, 12) | 0 | 2 | 🟢 |

**Issues:**
- {issue}

**🟢 PASSED** - {one line}
```

Status: 🟢 healthy; 🟡 consensus works but with issues; 🔴 consensus broken or a node's blocks fail
validation. End with one result line: `🟢 PASSED`, `🟡 PASSED WITH WARNINGS` or `🔴 FAILED: {reason}`.
Report issues, not what's working.
