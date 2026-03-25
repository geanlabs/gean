# Fork Analysis

Guide to identifying and analyzing blockchain forks during devnet runs.

## What is a Fork?

A fork occurs when clients disagree on which blocks form the canonical chain. This can happen due to:
- Block propagation delays
- Signature verification differences
- State transition disagreements
- Genesis state mismatches

## Detecting Forks

### Quick Check: Block Hash Comparison

```bash
# Compare block hashes at same slot across all clients
for slot in 1 2 3 4 5; do
  echo "=== Slot $slot ==="
  grep -h "slot=$slot[^0-9]" *.log | grep -oE "block_root=0x[a-f0-9]{16}" | sort -u
done
```

**If you see multiple different hashes → fork exists!**

### Check for Rejected Blocks

```bash
# gean - rejected blocks
grep -E "rejected gossip block|sync block rejected" gean_*.log

# ethlambda - failed block processing
grep "Failed to process block" ethlambda_*.log

# lantern - signature failures
grep "signature verification failed" lantern_*.log

# qlean - invalid signatures
grep "Invalid signatures for block" qlean_*.log
```

## Understanding Fork Types

### Canonical Fork
The longest valid chain that should be followed by all clients.

### Orphan Block
A valid block that was not included in the canonical chain (arrived too late or lost vote).

### Invalid Fork
Blocks rejected due to:
- Signature verification failure
- State transition error
- Invalid state root

## gean Fork Indicators

### Block Rejected
```
[WRN] sync block rejected slot=X block_root=0x... err=...
[WRN] rejected gossip block slot=X block_root=0x... err=...
```

### Parent Chain Issues
```
[WRN] parent state missing for gossip block slot=X block_root=0x...
```

### Sync Status
```
[INF] skipping validator duties while behind peers slot=X
```

## Tracing Parent-Child Relationships

```bash
# Get block tree for a specific slot
grep "slot=10" gean_0.log | grep -E "block_root|parent_root"

# Find orphan blocks (parent not in chain)
grep "parent_root" gean_0.log | sort | uniq -c | sort -rn
```

## Building Fork Structure

1. **Collect all block_root and parent_root pairs**
2. **Build directed graph of parent→child relationships**
3. **Identify which blocks are ancestors of head**
4. **Find blocks that are not in the canonical chain**

## Determining Fork Ownership

Which validators are on which fork?

```bash
# gean - check proposer indices
grep "block accepted" gean_0.log | grep "slot=5" | grep -oE "proposer=[0-9]+"

# lantern - rejected votes
grep "rejected vote" lantern_0.log | grep -oE "validator=[0-9]+" | sort | uniq -c
```

## Debugging Fork Issues

1. **Check finalization** - If finalization is stuck, may indicate fork
2. **Compare head blocks** - Different clients may have different heads
3. **Check attestations** - Attestations to unknown blocks indicate forks
4. **Verify signatures** - Signature verification differences cause forks

## Prevention

- Ensure consistent genesis state
- Implement proper block propagation
- Use batched requests (prevents sync storms)
- Monitor peer connections
