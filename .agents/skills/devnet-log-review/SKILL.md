---
name: devnet-log-review
description: Review and analyze devnet run results. Use when users want to (1) Analyze devnet logs for errors and warnings, (2) Generate a summary of a devnet run, (3) Identify interoperability issues between clients, (4) Understand consensus progress and block production, (5) Debug forks and finalization issues.
---

# Devnet Log Review

Analyze and summarize devnet run results from lean consensus testing involving
gean and peer clients. Client names, images, and ports live in
[devnet-runner/references/clients.md](../devnet-runner/references/clients.md).

## Quick Start

```bash
# From project root (with logs in current directory)
.agents/skills/devnet-log-review/scripts/analyze-logs.sh

# Or specify logs directory
.agents/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/logs
```

This prints error/warning counts, block counts, the last observed head,
justified, and finalized slot per node, proposer slots, and a verdict:

| Verdict | Exit | Meaning |
|---|---|---|
| HEALTHY | 0 | No errors, and every gean node log shows `[forkchoice] finalized advanced slot=` |
| UNHEALTHY | 1 | At least one node log has errors |
| UNKNOWN | 2 | Insufficient evidence: no node logs, no gean logs, empty logs, or no finalization on a gean node |

Only gean's log patterns are verified against source; other clients' nodes
show N/A in the tables and their progress is not judged. Their errors still count.

## Log File Locations

| File | Content |
|---|---|
| `devnet.log` | Combined output from `spin-node.sh` (genesis generation + all node output); skipped by the scripts |
| `{client}_{n}.log` | Individual node logs (e.g., `gean_0.log`, `zeam_0.log`) |

## Analysis Scripts

| Script | Description |
|---|---|
| `analyze-logs.sh [dir]` | Main entry point — runs all analyses, outputs markdown summary and verdict |
| `count-errors-warnings.sh [dir]` | Count errors/warnings per node (excludes benign patterns) |
| `count-blocks.sh [dir]` | Count blocks proposed/processed per gean node |
| `check-consensus-progress.sh [dir]` | Last head event per node (head, justified, finalized slot) and proposer slots |
| `show-errors.sh [-n node] [-l limit] [-w] [dir]` | Display error details for investigation |

After editing any script, run `scripts/test/run.sh`.

```bash
# Show errors for specific node
.agents/skills/devnet-log-review/scripts/show-errors.sh -n zeam_0

# Show errors and warnings with limit
.agents/skills/devnet-log-review/scripts/show-errors.sh -w -l 50
```

## Tracing Slot-by-Slot Flow

gean always emits ANSI color codes; strip them before matching:

```bash
sed -E 's/\x1b\[[0-9;]*m//g' gean_0.log | grep -E "slot=12([^0-9]|$)"
```

gean's structured fields use `key=value`:
- `slot=N` — Slot number
- `validator=N` — Validator index
- `proposer=N` — Block proposer index
- `justified_slot=N` / `finalized_slot=N` — Checkpoints at the time of the log
- `proc_time=X` — Block processing time
- `has_parent=true|false` — Whether the block's parent was already known
- `attestations=N` — Number of attestations in the block

## References

- [CLIENT_LOG_PATTERNS.md](references/CLIENT_LOG_PATTERNS.md) — gean's log lines (proposal and import flow, head events, chain status) and unverified patterns from other clients
- [FORK_ANALYSIS.md](references/FORK_ANALYSIS.md) — Rejected blocks, parent-child tracing, cross-client block hash comparison, validator IDs, reorgs
- [FINALIZATION_DEBUG.md](references/FINALIZATION_DEBUG.md) — Finalization stalls, the ≥ 2/3 threshold, the 3SF-mini gap rule
- [ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md) — Critical vs. benign errors and interoperability issues

## Summary Report Format

Generate concise summaries (20 lines or less) in this structure:

```markdown
## Devnet Log Summary

**Run:** {N} {client} nodes (`{image}`) | {M} slots ({range})

| Node | Validator | Blocks Proposed | Errors | Warnings | Status |
|---|---|---|---|---|---|
| {node_name} | {id} | {count} (slots {list}) | {n} | {n} | {verdict} |

**Issues:**
- {issue 1}
- {issue 2}

**{VERDICT}** — {one-line explanation}
```

Rules:
1. Use the HEALTHY / UNHEALTHY / UNKNOWN verdicts above; never report HEALTHY
   without progress evidence.
2. A node is UNHEALTHY if its blocks fail validation on peers.
3. Focus on issues; don't list what's working.
