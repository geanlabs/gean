---
name: devnet-log-review
description: Review and analyze a lean devnet run from gean's node logs. Use when asked to (1) analyze devnet logs for errors and warnings, (2) summarize a devnet run, (3) check block production and consensus progress, or (4) debug forks and stalled finalization.
---

# Devnet log review

Analyze and summarize a devnet run, typically one started with `make lean-devnet` or the
[`devnet-runner`](../devnet-runner/SKILL.md) skill.

## Quick start

```bash
.agents/skills/devnet-log-review/scripts/analyze-logs.sh            # logs in the current directory
.agents/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/logs
.agents/skills/devnet-log-review/scripts/analyze-logs.sh -e 20      # also each node's first 20 errors and warnings
```

Per node it reports blocks proposed and imported, the head, justified and finalized slots from the
last `CHAIN STATUS` block, and error and warning counts, then the total errors. It reads
`gean_<n>.log`; `devnet.log` holds lean-quickstart's combined output.

## gean's log lines

gean logs `timestamp LEVEL [component] message`, with components `chain`, `forkchoice`,
`validator`, `signature`, `gossip`, `network`, `sync`, `store` and `node`.

```
INFO [validator] proposed block slot=X block_root=0x... attestations=N     # a block this node built
INFO [chain] block slot=X ... justified_slot=J finalized_slot=F             # a block this node imported
INFO [forkchoice] head slot=X head_root=0x...                                # head moved
INFO [forkchoice] finalized advanced slot=X root=0x...                       # finality moved
INFO [signature] aggregate: slot=X raw=N ...                                 # aggregator built a proof
  Latest Justified:   Slot J | Root: 0x...                                   # in the periodic CHAIN STATUS block
  Latest Finalized:   Slot F | Root: 0x...
```

A healthy proposal: `proposing block slot=X validator=Y`, then `proposed block slot=X ...`. The
warning and error lines worth knowing are in
[references/ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md).

## Common investigations

```bash
# Everything about one slot, across nodes
grep -hE "slot=12([^0-9]|$)" gean_*.log

# Block roots at a slot: more than one means a fork
grep -h "slot=4[^0-9]" gean_*.log | grep -oE "block_root=0x[a-f0-9]{16}" | sort -u

# Finality over time on one node
grep "Latest Finalized" gean_0.log | tail -20
```

A validator proposes when `slot % validator_count == validator_index`.

**Justification needs a two-thirds supermajority**, `3 * votes >= 2 * validator_count`
(`internal/lean/transition/attestations.go`): 6 validators need 4 votes, 9 need 6, 16 need 11.
At least one node must aggregate (`--is-aggregator`), or blocks carry no attestations and
nothing is ever justified.

Deeper guides, loaded only when needed:

- [references/FORK_ANALYSIS.md](references/FORK_ANALYSIS.md): forks, parents, rejected blocks,
  reorgs, and nodes on different heads.
- [references/FINALIZATION_DEBUG.md](references/FINALIZATION_DEBUG.md): causes of finality
  stalls and the justification chain.
- [references/ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md): critical, medium and
  benign log lines.

## Summary format

Keep it under 20 lines:

```markdown
## Devnet log summary

**Run:** {N} nodes | {M} slots ({range})

| Node | Validators | Blocks proposed | Finalized | Errors | Warnings | Status |
|---|---|---|---|---|---|---|
| gean_0 | 0, 3 | 4 (slots 3, 6, 9, 12) | 9 | 0 | 2 | 🟢 |

**Issues:**
- {issue}

**🟢 PASSED** - {one line}
```

Status: 🟢 healthy; 🟡 consensus works but with issues; 🔴 consensus broken or a node's blocks
fail validation. End with one result line: `🟢 PASSED`, `🟡 PASSED WITH WARNINGS` or
`🔴 FAILED: {reason}`. Report issues, not what's working.
