# Fork Analysis Guide

Comprehensive guide to identifying and analyzing blockchain forks in devnet runs.

## Understanding Forks

**Fork Types:**
1. **Canonical Fork** — The main chain that the honest majority follows
2. **Orphan Fork** — Valid blocks that lost a fork choice race (e.g., two
   blocks proposed for same slot)
3. **Invalid Fork** — Chain built on blocks with validation failures
   (signature errors, state errors, etc.)

**Key Insight:** Blocks don't just have slot numbers — they have **parent
relationships**. A fork occurs when blocks at different slots reference
different parent blocks.

## Tracing Parent-Child Relationships

To understand forks, map out the blockchain DAG (Directed Acyclic Graph) by
tracking which block is the parent of each new block.

### gean — Explicit Parent Logging

```bash
# gean logs parent relationships in [chain] block lines
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "\[chain\] block slot=" | head -20
# Output: [chain] block slot=12 block_root=0x... parent_root=0x... proposer=3
#         attestations=4 justified_slot=10 finalized_slot=6 proc_time=88ms

# gean's gossip layer shows incoming blocks
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "\[gossip\] received block" | head -20

# When parent is missing, gean logs and stores as pending
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "block parent missing"
# Output: block parent missing slot=N block_root=0x... parent_root=0x... depth=D, storing as pending
```

### ethlambda — Pending Blocks

```bash
# When ethlambda receives a block with unknown parent:
grep "Block parent missing" ethlambda_0.log
# Output: Block parent missing, storing as pending slot=8 parent_root=0x... block_root=0x...
#         Meaning: slot 8 block depends on parent 0x... which ethlambda doesn't have

# Check processed blocks
grep "Processed new block\|Fork choice head updated" ethlambda_0.log | head -20
```

### lantern — Import Logs

```bash
grep "imported block" lantern_0.log | head -20
# Output: imported block slot=3 new_head_slot=3 head_root=0x...
```

### zeam — Block Processing

```bash
sed 's/\x1b\[[0-9;]*m//g' zeam_0.log | grep "processing block\|imported block\|processed block" | head -20
```

### ream — Block Processing Service

```bash
grep "Processing block built\|Fork choice head updated" ream_0.log | head -20
```

## Building the Fork Structure

### Step 1: Map Canonical Chain

Start from genesis and follow the longest/heaviest chain:

```bash
# For gean — extract block roots in slot order
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "\[chain\] block slot=" | \
  grep -oE "slot=[0-9]+|block_root=0x[a-f0-9]{8}" | paste - - | head -30

# Compare block hashes at each slot across clients
# If clients have different hashes at same slot → fork!
```

### Step 2: Identify Rejected Blocks

```bash
# Find blocks rejected by signature verification
grep -i "signature.*failed\|invalid signature" *.log

# gean
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "block processing failed"

# ethlambda
grep "Failed to process block" ethlambda_0.log
# Output: Failed to process block slot=4 err=Proposer signature verification failed

# lantern
grep "signature verification failed\|rejected" lantern_0.log
```

### Step 3: Track Attestations to Unknown Blocks

Attestations reference blocks by hash. If a client receives attestations for
an unknown block, it indicates a fork:

```bash
# ethlambda logs "Unknown head block" or "Unknown target block"
grep "Unknown.*block:" ethlambda_0.log | head -20

# Count attestations per unknown block
grep "Unknown.*block:" ethlambda_0.log | grep -oE "0x[a-f0-9]{64}" | sort | uniq -c | sort -rn

# ream version
grep "Unknown.*block\|No state available for target" ream_0.log | grep -oE "0x[a-f0-9]{64}" | sort -u
```

### Step 4: Determine Which Validators Are on Which Fork

```bash
# Check who is attesting to rejected blocks (lantern is most expressive here)
grep "rejected vote\|rejected attestation" lantern_0.log | grep "validator=" | head -20

# Cross-check with each node's reported head
for f in *.log; do
    node=$(basename "$f" .log)
    head_line=$(sed 's/\x1b\[[0-9;]*m//g' "$f" | grep -E "Head Block Root|Head Slot" | tail -1)
    echo "$node: $head_line"
done
```

## Fork Structure Diagram Format

When you identify forks, document them in ASCII:

```
                           GENESIS (slot 0)
                           0xc8849d39...
                                │
              ┌─────────────────┴─────────────────┐
              │                                    │
          SLOT 1 █                             SLOT 4 ✗
       0xcbe3c545...                        0xa829bac5...
    ┌─────────────────┐                   (INVALID — rejected
    │  CANONICAL (A)  │                    by 3/4 clients)
    │  Clients:       │                          │
    │  ✓ gean         │                     SLOT 10 ⚠
    │  ✓ ethlambda    │                    0xf8dae5ee...
    │  ✓ zeam         │                  (invalid fork, only
    │  ✓ lantern      │                   ream follows)
    └─────────────────┘
              │
          SLOT 3 █
       0x0c3dd6a5...
              │
          SLOT 5 █
       0xd0fd6225...
              │
        (continues...)

Legend:
  █ = Canonical block    ✗ = Rejected block    ⚠ = Block on invalid fork
```

## Key Questions to Answer

1. **Which block(s) were rejected and why?** (signature errors, state errors,
   etc.)
2. **Which validators accepted the rejected block?** (check their heads)
3. **How many validators are on each fork?** (count unique attestations per
   fork)
4. **Can the canonical fork finalize without the validators on invalid fork?**
   (need >2/3 supermajority)

## Signature Verification Disagreements

If clients disagree on signature validity, determine consensus:

```bash
# Count how many clients rejected vs accepted a specific block
BLOCK_HASH="0xa829bac56f6b98fbe16ed02cde4166a0a0df2e68c68e64afa4fce43bbe1992b3"

echo "=== Clients that rejected $BLOCK_HASH ==="
grep -l "signature.*failed.*$BLOCK_HASH\|Invalid signatures.*$BLOCK_HASH" *.log

echo "=== Clients that accepted $BLOCK_HASH ==="
grep -l "Processed.*$BLOCK_HASH\|imported.*$BLOCK_HASH\|\[chain\] block.*$BLOCK_HASH" *.log

# If 3/4 clients reject → the block is genuinely invalid, bug in proposer
# If 1/4 clients reject → possible bug in that client's validation
```

### Root Cause Determination

- If **majority rejects** with signature errors → **proposer has bug**
  (failed to sign properly)
- If **minority rejects** with signature errors → **validator has bug**
  (incorrect validation)
- If **different blocks at same slot** → fork choice race (benign, resolved by
  fork choice)

## Comparing Block Hashes Across Slots

```bash
# Extract block hashes for specific slots (comparing across clients)
for slot in 1 2 3 4 5; do
  echo "=== Slot $slot ==="
  sed 's/\x1b\[[0-9;]*m//g' *.log | grep -h "slot=$slot[^0-9]" | grep -oE "0x[a-f0-9]{8}" | sort -u
done

# Check which client has which head at a specific slot
grep -h "Head Slot: 18\|head slot=18" *.log

# Compare finalization across clients
grep -h "finalized.*slot\|Finalized block.*@\|finalized_slot=" *.log | tail -20
```

## Validator ID Detection

Each validator proposes blocks when `slot % validator_count == validator_id`.

### Finding Validator IDs from Logs

```bash
# gean — explicit validator in [validator] log lines
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "produced attestation\|proposing block" | head -3
# Output: [validator] produced attestation slot=6 validator=2
# Pattern: validator=2 proposes at slots 2, 7, 12, ... (every 5th if 5 validators)

# ethlambda — explicit validator_id
grep "We are the proposer" ethlambda_0.log | head -3
# Output: We are the proposer for this slot slot=5 validator_id=5

# zeam — proposer field
grep "packing proposer attestation" zeam_0.log | head -3
# Output: packing proposer attestation for slot=6 proposer=0

# Generic — validator_id = slot % validator_count
```

### Verify Validator Count

```bash
# Count unique validators from attestations
grep -h "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u | wc -l

# Or check genesis/validator-config.yaml for the configured count
```

## Reorg Detection

Reorgs are normal — they happen when fork choice swaps the head between
competing branches. They become a problem only when frequent or deep.

```bash
# gean logs REORG explicitly
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "REORG"
# Output: REORG slot=N head_root=0x... (was 0x...) justified_slot=J finalized_slot=F

# Count reorgs per node
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep -c "REORG"
```

A handful of single-slot reorgs (1 reorg per ~50 blocks) is normal. Many
deep reorgs suggest a network partition or aggressive fork choice churn.
