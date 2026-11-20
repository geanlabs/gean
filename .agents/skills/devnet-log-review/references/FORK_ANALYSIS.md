<!-- Adapted from ethlambda (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Fork Analysis Guide

Comprehensive guide to identifying and analyzing blockchain forks in devnet runs.

## Understanding Forks

**Fork Types:**
1. **Canonical Fork** - The main chain that the honest majority follows
2. **Orphan Fork** - Valid blocks that lost a fork choice race (e.g., two blocks proposed for same slot)
3. **Invalid Fork** - Chain built on blocks with validation failures (signature errors, state errors, etc.)

**Key Insight:** Blocks don't just have slot numbers - they have **parent relationships**. A fork occurs when blocks at different slots reference different parent blocks.

## Tracing Parent-Child Relationships

To understand forks, map out the blockchain DAG (Directed Acyclic Graph) by tracking which block is the parent of each new block.

### qlean - Explicit Parent Logging

```bash
# qlean logs parent relationships when receiving blocks
grep "Received block.*parent=" qlean_0.log | head -20
# Output: Received block 0xabc...123 @ 3 parent=0xdef...456 from peer=...
# Meaning: slot 3 block (0xabc...123) builds on parent (0xdef...456)

# Also check "Added block" logs
grep "Added block.*as child of" qlean_0.log | head -20
# Output: Added block 0xabc...123 @ 3 as child of def4…5678
```

### gean - Missing Parents

```bash
# gean fetches a missing parent by root and holds the child until it arrives:
grep "queueing missing block\|has_parent=false" gean_0.log
# A parent that never arrives drops its children:
grep "fetch exhausted" gean_0.log
```

### ethlambda - Pending Blocks

```bash
# When ethlambda receives a block with unknown parent:
grep "Block parent missing" ethlambda_0.log
# Output: Block parent missing, storing as pending slot=8 parent_root=0x6cc163e6... block_root=0x16d1daad...
# Meaning: slot 8 block depends on parent 0x6cc163e6... which ethlambda doesn't have

# Check processed blocks
grep "Processed new block" ethlambda_0.log | head -20
# Shows which blocks were successfully validated and added to chain
```

### lantern - Import Logs

```bash
grep "imported block" lantern_0.log | head -20
# Output: imported block slot=3 new_head_slot=3 head_root=0x0c3dd6a5...
```

### zeam - Block Processing

```bash
sed 's/\x1b\[[0-9;]*m//g' zeam_0.log | grep "processing block\|imported block" | head -20
```

## Building the Fork Structure

### Step 1: Map Canonical Chain

Start from genesis and follow the longest/heaviest chain:

```bash
# For each client, extract processed blocks in order
grep "Processed new block\|imported block\|Added block" CLIENT.log | \
  grep -oE "slot=[0-9]+|block_root=0x[a-f0-9]{8}" | \
  paste - - | head -30

# Compare block hashes at each slot across clients
# If clients have different hashes at same slot → fork!
```

### Step 2: Identify Rejected Blocks

```bash
# Find blocks rejected by signature verification
grep -i "signature.*failed\|invalid signature" *.log

# gean
grep "block processing failed" gean_0.log

# ethlambda
grep "Failed to process block" ethlambda_0.log
# Output: Failed to process block slot=4 err=Proposer signature verification failed

# qlean
grep "Invalid signatures for block" qlean_0.log
# Output: Invalid signatures for block 0xa829bac5... @ 4

# lantern
grep "signature verification failed" lantern_0.log
# Output: signature verification failed slot=4 root=0xa829bac5...
```

### Step 3: Track Attestations to Unknown Blocks

Attestations reference blocks by hash. If a client receives attestations for an unknown block, it indicates a fork:

```bash
# ethlambda logs "Unknown head block" or "Unknown target block"
grep "Unknown.*block:" ethlambda_0.log | head -20
# Output: Failed to process gossiped attestation err=Unknown head block: 0xa829bac5...

# Count attestations per unknown block
grep "Unknown.*block:" ethlambda_0.log | grep -oE "0x[a-f0-9]{64}" | sort | uniq -c | sort -rn
# Output: 48 0x66adc5361a72c49aab91f28c3350734f6224e674fc39518416f2ef932f9523ae
#         12 0xa829bac56f6b98fbe16ed02cde4166a0a0df2e68c68e64afa4fce43bbe1992b3
# Many attestations for the same unknown block → multiple validators on that fork
```

### Step 4: Determine Which Validators Are on Which Fork

```bash
# Check who is attesting to rejected blocks
grep "rejected vote" lantern_0.log | grep "validator=" | head -20
# Output: rejected vote validator=4 slot=5 head=0xa829bac5... reason=unknown head
# Meaning: validator 4 is attesting to the rejected block at slot 4

# Check validator's own head
grep "head.*slot\|Head Block Root" grandine_0.log | head -10
# If grandine's head is the rejected block, grandine is on the invalid fork
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
    ┌─────────────────┐                   (INVALID - rejected
    │  CANONICAL (A)  │                    by 3/4 clients)
    │  Clients:       │                          │
    │  ✓ ethlambda    │                     SLOT 10 ⚠
    │  ✓ zeam         │                    0xf8dae5ee...
    │  ✓ lantern      │                  (invalid fork, only
    │  ✓ qlean        │                   grandine follows)
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

1. **Which block(s) were rejected and why?** (signature errors, state errors, etc.)
2. **Which validators accepted the rejected block?** (check their heads)
3. **How many validators are on each fork?** (count unique attestations per fork)
4. **Can the canonical fork finalize without the validators on invalid fork?** (need >2/3 supermajority)

## Signature Verification Disagreements

If clients disagree on signature validity, determine consensus:

```bash
# Count how many clients rejected vs accepted a specific block
BLOCK_HASH="0xa829bac56f6b98fbe16ed02cde4166a0a0df2e68c68e64afa4fce43bbe1992b3"

echo "=== Clients that rejected $BLOCK_HASH ==="
grep -l "signature.*failed.*$BLOCK_HASH\|Invalid signatures.*$BLOCK_HASH" *.log

echo "=== Clients that accepted $BLOCK_HASH ==="
grep -l "Processed.*$BLOCK_HASH\|imported.*$BLOCK_HASH" *.log

# If 3/4 clients reject → the block is genuinely invalid, bug in proposer
# If 1/4 clients reject → possible bug in that client's validation
```

### Root Cause Determination

- If **majority rejects** with signature errors → **proposer has bug** (failed to sign properly)
- If **minority rejects** with signature errors → **validator has bug** (incorrect validation)
- If **different blocks at same slot** → fork choice race (benign, resolved by fork choice)

## Comparing Block Hashes Across Slots

```bash
# Extract block hashes for specific slots (comparing across clients)
for slot in 1 2 3 4 5; do
  echo "=== Slot $slot ==="
  grep -h "slot=$slot[^0-9]\|@ $slot[^0-9]" *.log | grep -oE "0x[a-f0-9]{8}" | sort -u
done

# Check which client has which head at a specific slot
grep -h "head_slot=18\|Head Slot: 18" *.log

# Compare finalization across clients
grep -h "finalized.*slot\|Finalized block.*@" *.log | tail -20
```

## Validator ID Detection

Each validator proposes blocks when `slot % validator_count == validator_id`.

### Finding Validator IDs from Logs

```bash
# gean - explicit validator
grep "proposing block" gean_0.log | head -3

# ethlambda - explicit validator_id
grep "We are the proposer" ethlambda_0.log | head -3
# Output: We are the proposer for this slot slot=5 validator_id=5
# Pattern: validator_id=5 proposes at slots 5,11,17,23... (every 6th if 6 validators)

# zeam - proposer field
grep "packing proposer attestation" zeam_0.log | head -3
# Output: packing proposer attestation for slot=6 proposer=0
# Pattern: proposer=0 proposes at slots 0,6,12,18...

# grandine - check proposal slots
grep "Using parent root for block proposal" grandine_0.log
# If it proposes at slots 4,10,16,22... then validator_id=4

# Generic - validator_id = slot % validator_count
```

### Verify Validator Count

```bash
# Count unique validators from attestations
grep -h "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u | wc -l

# Or check genesis configuration
grep "GENESIS_VALIDATORS" genesis/genesis.json | jq '. | length'
```
