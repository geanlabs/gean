<!-- Adapted from ethlambda (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Finalization Debugging Guide

Guide for diagnosing and debugging finalization issues in devnet runs.

## What is Finalization?

Finalization is the process by which slots become irreversible in the blockchain. In the lean consensus protocol (3SF-mini), finalization requires:
- >2/3 supermajority of validators attesting
- Proper justification chain (slots justified at specific intervals)

## Checking Finalization Progress

```bash
# Track finalization over time for each client
grep -h "finalized.*slot\|Finalized block.*@" *.log | tail -50

# gean
grep "Latest Finalized" gean_0.log | tail -20

# ethlambda specific
grep "finalized_slot=" ethlambda_0.log | tail -20

# qlean specific
grep "Finalized block" qlean_0.log | tail -20

# grandine specific
grep "Finalized Slot:" grandine_0.log | tail -20
```

**Expected pattern:** Finalization should advance roughly every 6-12 slots (depending on 3SF-mini rules).

**Stall indicator:** Finalized slot stays the same for 50+ slots while head slot continues advancing.

## Example of Healthy Finalization

```
Slot 0:  finalized_slot=0
Slot 6:  finalized_slot=0  (waiting for justification)
Slot 12: finalized_slot=6  (slot 6 finalized)
Slot 18: finalized_slot=12 (slot 12 finalized)
Slot 24: finalized_slot=18 (slot 18 finalized)
```

## Example of Finalization Stall

```
Slot 0:  finalized_slot=0
Slot 6:  finalized_slot=0
Slot 12: finalized_slot=6
Slot 18: finalized_slot=12
Slot 24: finalized_slot=18  ← finalized
Slot 30: finalized_slot=18  ← STUCK
Slot 50: finalized_slot=18  ← STILL STUCK
Slot 100: finalized_slot=18 ← NOT ADVANCING
```

## Common Causes of Finalization Stalls

### 1. Insufficient Validator Participation

**Requirement:** Need **>2/3 supermajority** to finalize
- With 6 validators: need >4 votes = **at least 5 votes**
- With 9 validators: need >6 votes = **at least 7 votes**

If validators are on different forks, neither fork may reach >2/3.

```bash
# Count how many validators are active (attesting)
grep "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u

# Check which validators are on which fork (by head block they attest to)
grep "head=0x" lantern_0.log | grep "validator=" | tail -30
```

### 2. Validators on Invalid Fork

If N validators follow an invalid fork, only (total - N) validators contribute to canonical chain.

**Example:** 6 validators, 1 on invalid fork
- Total: 6 validators
- Honest: 5 validators on canonical fork
- Threshold: >4 votes, so need 5 votes
- Available: 5 honest votes
- **Should finalize!** 5 > 4 ✓

**Example:** 6 validators, 2 on invalid fork
- Total: 6 validators
- Honest: 4 validators on canonical fork
- Threshold: >4 votes, so need 5 votes
- Available: 4 honest votes
- **Cannot finalize!** 4 ≯ 4 ✗

```bash
# Find which validators are following invalid blocks
grep "rejected vote" lantern_0.log | grep -oE "validator=[0-9]+" | sort | uniq -c

# If validator 4 keeps getting rejected, validator 4 is on wrong fork
```

### 3. Missing Attestations

Client fails to process attestations from certain validators.

```bash
# Check for attestation processing failures
grep "Failed to process.*attestation" ethlambda_0.log | tail -30

# Common reasons:
# - "Unknown head block" → validator attesting to block this client doesn't have
# - "Unknown target block" → validator attesting to invalid/orphan fork blocks
```

**Impact:**
- Missing attestations reduce effective vote count
- May prevent reaching >2/3 threshold even if enough validators are on canonical fork

### 4. Justification Chain Broken

3SF-mini requires justified slots at specific intervals:
- Delta ≤ 5 from finalized slot
- Perfect squares (9, 16, 25, 36...)
- Pronic numbers (6, 12, 20, 30...)

Missing blocks or attestations can break justification chain.

```bash
# Check justification progress (gean)
grep "Latest Justified" gean_0.log | tail -30

# Check justification progress (ethlambda specific)
grep "latest_justified\|justified.*slot" ethlambda_0.log | tail -30

# Look for gaps in justified slots
```

## Finalization Math

Given:
- `N` = total validators
- `N_honest` = validators on canonical fork
- `N_invalid` = validators on invalid/wrong fork
- Threshold = **> 2N/3** votes needed (strictly greater than 2/3)

### Examples

**6 validators, 1 on invalid fork:**
- Total: 6 validators
- Honest: 5 validators on canonical fork
- Threshold: > 2×6/3 = > 4, so need **at least 5 votes**
- Available honest votes: 5
- **Should finalize!** 5 > 4 ✓

**6 validators, 2 on invalid fork:**
- Total: 6 validators
- Honest: 4 validators on canonical fork
- Threshold: > 4, so need **at least 5 votes**
- Available honest votes: 4
- **Cannot finalize!** 4 ≯ 4 ✗ (exactly 2/3 is not enough)

**6 validators, 1 crashed + 1 on invalid fork:**
- Total: 6 validators
- Honest: 4 validators on canonical fork
- Threshold: > 4, so need **at least 5 votes**
- Available honest votes: 4
- **Cannot finalize!** Network stuck until validators come back or rejoin canonical fork

## Debugging Steps

### Step 1: Verify Validator Count and Status

```bash
# Count total validators
grep -h "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u | wc -l

# Check which nodes are proposing blocks (active validators)
grep -h "We are the proposer\|Using parent root" *.log | head -30
```

### Step 2: Check Fork Structure

```bash
# See if clients have different heads
grep -h "head_slot=30\|Head Slot: 30" *.log

# Compare block hashes at recent slots
for slot in 28 29 30 31 32; do
  echo "=== Slot $slot ==="
  grep -h "slot=$slot[^0-9]\|@ $slot[^0-9]" *.log | grep -oE "0x[a-f0-9]{8}" | sort -u
done
```

### Step 3: Count Attestations

```bash
# Count attestations received per slot (ethlambda)
grep "Received new attestation.*slot=30" ethlambda_0.log | wc -l

# Expected: N-1 attestations per slot (all validators except proposer)
# With 6 validators: expect 5 attestations per slot
```

### Step 4: Check for Processing Failures

```bash
# Look for attestation processing failures
grep "Failed to process.*attestation" ethlambda_0.log | tail -50

# Group by error type
grep "Failed to process.*attestation" ethlambda_0.log | \
  grep -oE "err=.*" | sort | uniq -c
```

### Step 5: Verify Threshold Calculation

```bash
# Calculate if finalization should be possible
echo "Total validators: 6"
echo "Threshold: > 2×6/3 = > 4, need 5 votes"
echo "Validators on canonical fork: ?"  # Count from logs
echo "Can finalize: yes if ≥5, no if ≤4"
```

## Known Bugs

### Bug: Waiting for All Validators

**Symptom:** Finalization stalls even with >2/3 validators on canonical fork

**Cause:** Finalization logic waits for attestations from ALL validators instead of just >2/3

**Example:** 6 validators, 1 on invalid fork
- Available: 5 votes from honest validators
- Threshold: need 5 votes
- Bug behavior: waits for 6th validator (on invalid fork) which will never attest
- Expected behavior: should finalize with 5 votes

**Detection:**
```bash
# Check if stalled client has enough attestations
# If yes but finalization stalled → possible bug
```

### Bug: Off-by-One in Threshold

**Symptom:** Finalization requires exactly 2/3 instead of >2/3

**Cause:** Using `>=` instead of `>` in threshold check

**Detection:** Check if finalization succeeds with exactly 4/6 votes but protocol requires >4

## Additional Resources

See [FORK_ANALYSIS.md](FORK_ANALYSIS.md) for fork detection and [ERROR_CLASSIFICATION.md](ERROR_CLASSIFICATION.md) for common error patterns.
