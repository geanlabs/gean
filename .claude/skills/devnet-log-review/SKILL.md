---
name: devnet-log-review
description: Review and analyze devnet run results for gean client. Use when users want to (1) Analyze devnet logs for errors and warnings, (2) Generate a summary of a devnet run, (3) Identify interoperability issues between clients, (4) Understand consensus progress and block production, (5) Debug forks and finalization issues.
---

# Devnet Log Review (gean)

Analyze and summarize devnet run results from lean consensus testing.

## Quick Start

**Run the analysis script:**
```bash
# From project root (with logs in current directory)
./.claude/skills/devnet-log-review/scripts/analyze-logs.sh

# Or specify logs directory
./.claude/skills/devnet-log-review/scripts/analyze-logs.sh /path/to/logs
```

This produces a structured summary with:
- Error/warning counts per node
- Block production statistics
- Consensus progress (head slot, finalized slot, justified slot)
- Sync status

## Log File Locations

| File | Content |
|------|---------|
| `devnet.log` | Combined output from genesis generation + all node output |
| `gean_{n}.log` | Individual node logs (e.g., `gean_0.log`, `gean_1.log`) |

## Analysis Scripts

| Script | Description |
|--------|-------------|
| `analyze-logs.sh [dir]` | Main entry point - runs all analyses, outputs markdown summary |
| `count-errors-warnings.sh [dir]` | Count errors/warnings per node (excludes benign patterns) |
| `count-blocks.sh [dir]` | Count blocks proposed/processed per node |
| `check-consensus-progress.sh [dir]` | Show last slot reached and consensus state |
| `show-errors.sh [-n node] [-l limit] [-w] [dir]` | Display error details for investigation |

**Usage examples:**
```bash
# Just count errors/warnings
./.claude/skills/devnet-log-review/scripts/count-errors-warnings.sh

# Show errors for specific node
./.claude/skills/devnet-log-review/scripts/show-errors.sh -n gean_0

# Show errors and warnings with limit
./.claude/skills/devnet-log-review/scripts/show-errors.sh -w -l 50
```

## gean Log Format

gean uses structured logging with key=value format. Key fields:

### Timing
- `slot=N` - Current slot number
- `head_slot=N` - Head block slot
- `finalized_slot=N` - Latest finalized slot
- `justified_slot=N` - Latest justified slot

### Block/Attestation
- `proposer=N` - Block proposer index
- `validator=N` - Validator index
- `block_root=0x...` - Full block hash (64 hex chars)
- `parent_root=0x...` - Parent block hash
- `attestations=N` - Number of attestations in block

### Network/Sync
- `peer_id=...` - LibP2P peer ID
- `peers=N` - Connected peer count
- `behind_peers=true/false` - Sync status
- `max_peer_head=N` - Highest peer head slot

### Errors
- `err=...` - Error message
- `WRN` level - Warnings
- `ERR` level - Errors

## Common Investigation Patterns

### Tracing Slot-by-Slot Flow

**Note:** Logs contain ANSI color codes. Strip them first:

```bash
# Strip ANSI codes and grep for a specific slot
sed 's/\x1b\[[0-9;]*m//g' devnet.log | grep "slot=3[^0-9]"

# For double-digit slots
sed 's/\x1b\[[0-9;]*m//g' devnet.log | grep "slot=12[^0-9]"
```

### Comparing Clients at Specific Slots

```bash
# Extract block hashes for specific slots across all clients
for slot in 1 2 3 4 5; do
  echo "=== Slot $slot ==="
  grep -h "slot=$slot[^0-9]" *.log | grep -oE "0x[a-f0-9]{16}" | sort -u
done

# Check which client has which head at a specific slot
grep -h "head_slot=18" *.log

# Compare finalization across clients
grep -h "finalized_slot=" *.log | tail -20
```

### Finding Block Proposals

```bash
# gean - block accepted logs
grep "block accepted" gean_0.log | head -5

# gean - synced block during sync
grep "synced block" gean_0.log | head -5

# gean - proposer logs
grep "proposer" gean_0.log | head -5
```

## Analysis Areas

### Fork Analysis

When clients disagree on which blocks are valid, the network splits into forks.

**Quick check for forks:**
```bash
# Compare block hashes at same slot across clients
grep -h "slot=4[^0-9]" *.log | grep -oE "block_root=0x[a-f0-9]{16}" | sort -u

# If you see different hashes → fork exists!
```

**Identifying rejected blocks:**
```bash
# Check for rejected gossip blocks
grep "rejected gossip block" gean_0.log | head -20

# Check for sync block rejections
grep "sync block rejected" gean_0.log | head -20

# Check for parent state missing
grep "parent state missing" gean_0.log | head -20
```

**See [references/FORK_ANALYSIS.md](references/FORK_ANALYSIS.md) for:**
- Understanding fork types (canonical, orphan, invalid)
- Tracing parent-child relationships
- Building fork structure diagrams
- Determining which validators are on which fork

### Sync Debugging

gean logs sync progress with detailed information:

```bash
# Check sync progress
grep "starting sync walk\|batch request\|batch response\|synced block\|sync with peer completed" gean_0.log

# Check for behind peers warnings
grep "skipping validator duties while behind peers" gean_0.log

# Check BlocksByRoot requests (reduced after batching)
grep "blocks_by_root" gean_0.log | head -20
```

**See [references/SYNC_DEBUG.md](references/SYNC_DEBUG.md) for:**
- Understanding batched requests vs individual requests
- Detecting sync storms
- Recovery from parent state missing

### Finalization Debugging

Finalization should advance every 6-12 slots. If it stalls, investigate:

```bash
# Check finalization progress
grep "finalized_slot=" gean_0.log | tail -20

# If finalized_slot stays same for 50+ slots → finalization stalled
```

**Finalization requires >2/3 supermajority:**
- 6 validators → need 5 votes minimum
- 9 validators → need 7 votes minimum

**See [references/FINALIZATION_DEBUG.md](references/FINALIZATION_DEBUG.md) for:**
- Common causes of finalization stalls
- Validator participation calculations
- Justification chain analysis
- Step-by-step debugging guide

### Error Classification

**See [references/ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md) for:**
- Critical errors (genesis mismatch, panics, database corruption)
- Expected/benign messages (TODOs, connection timeouts to unconfigured nodes)
- Medium severity issues (encoding mismatches, missing blocks)
- State transition errors

### Client Log Patterns

Different clients have different log formats and key patterns.

**See [references/CLIENT_LOG_PATTERNS.md](references/CLIENT_LOG_PATTERNS.md) for:**
- Log format for each client (zeam, ream, ethlambda, grandine, lantern, qlean)
- Key log patterns per client
- Block counting methods
- ANSI color code handling

## Summary Report Format

Generate concise summaries (20 lines or less) in this structure:

```markdown
## Devnet Log Summary

**Run:** {N} gean nodes | {M} slots ({range})

| Node | Blocks Processed | Sync Status | Errors | Warnings | Status |
|------|-----------------|-------------|--------|----------|--------|
| gean_{n} | {count} | {synced/behind} | {n} | {n} | {emoji} |

**Consensus State:**
- Head slot: {slot}
- Finalized slot: {slot}
- Justified slot: {slot}

**Issues:**
- {issue 1}
- {issue 2}

**{emoji} {RESULT}** - {one-line explanation}
```

### Status Emoji Guide

| Emoji | Meaning | When to Use |
|-------|---------|-------------|
| 🟢 | Healthy | No errors, consensus working |
| 🟡 | Warning | Minor issues but consensus working |
| 🔴 | Failed | Critical errors, consensus broken |

### Result Line Examples

- `🟢 PASSED` - All nodes healthy, consensus achieved
- `🟡 PASSED WITH WARNINGS` - Consensus working but minor issues detected
- `🔴 FAILED` - Consensus broken: {reason}

### Key Rules

1. Keep summary under 20 lines
2. Use table for per-node status
3. Status should reflect whether consensus is working (🔴 if not)
4. End with single-line result with emoji
5. Don't list "what's working" - focus on issues

## Manual Investigation Commands

Use these when scripts don't provide enough detail:

```bash
# Find block production
grep -h "block accepted\|synced block" gean_*.log | wc -l

# Check peer connections
grep -h "peer connected\|peer disconnected" gean_*.log | head -20

# Check attestations
grep -i "attestation" gean_*.log | head -50

# Search for specific error patterns
grep -i "genesis mismatch\|panic\|fatal" gean_*.log

# Check sync status
grep "skipping validator duties" gean_*.log | wc -l

# Check BlocksByRoot activity (should be batched after fix)
grep "blocks_by_root" gean_*.log | head -20
```

## Detailed References

For in-depth analysis, see these specialized guides:

- **[FORK_ANALYSIS.md](references/FORK_ANALYSIS.md)** - Comprehensive guide to identifying and analyzing blockchain forks
- **[FINALIZATION_DEBUG.md](references/FINALIZATION_DEBUG.md)** - Debugging finalization stalls
- **[CLIENT_LOG_PATTERNS.md](references/CLIENT_LOG_PATTERNS.md)** - Log formats for all clients
- **[ERROR_CLASSIFICATION.md](references/ERROR_CLASSIFICATION.md)** - Error types and severity levels
- **[SYNC_DEBUG.md](references/SYNC_DEBUG.md)** - Sync debugging, batched requests, request storms

## Progressive Disclosure

This skill uses progressive disclosure to keep context usage efficient:

1. **Start here** (SKILL.md) - Quick start workflow and common patterns
2. **Detailed references** (references/*.md) - Deep dives into specific analysis areas
3. **Scripts** (scripts/) - Automated analysis tools

Load detailed references only when needed for specific investigations.
