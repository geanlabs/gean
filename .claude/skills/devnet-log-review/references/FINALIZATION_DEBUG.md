# Finalization Debugging Guide

Guide for diagnosing and debugging finalization issues in devnet runs.

## What is Finalization?

Finalization is the process by which slots become irreversible in the
blockchain. In the lean consensus protocol (3SF-mini), finalization requires:
- >2/3 supermajority of validators attesting (technically `3 * votes >= 2 * total`)
- Justification chain with no "justifiable but unjustified" gaps between source
  and target

## Checking Finalization Progress

```bash
# Track finalization over time per client
grep -h "finalized.*slot\|Finalized block.*@\|finalized_slot=" *.log | tail -50

# gean specific
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "finalized_slot=" | tail -20
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "Latest Finalized:" | tail -10

# ethlambda specific
grep "finalized_slot=" ethlambda_0.log | tail -20

# ream specific
grep "Finalization\|finalized" ream_0.log | tail -20

# lantern specific
grep "finalized" lantern_0.log | tail -20
```

**Expected pattern:** Finalization should advance every 6-12 slots (depending
on 3SF-mini gap rules).

**Stall indicator:** Finalized slot stays the same for 50+ slots while head
slot continues advancing.

## Example of Healthy Finalization

```
Slot 0:   finalized_slot=0
Slot 6:   finalized_slot=0  (waiting for justification)
Slot 12:  finalized_slot=6  (slot 6 finalized)
Slot 18:  finalized_slot=12 (slot 12 finalized)
Slot 24:  finalized_slot=18 (slot 18 finalized)
```

## Example of Finalization Stall

```
Slot 0:   finalized_slot=0
Slot 6:   finalized_slot=0
Slot 12:  finalized_slot=6
Slot 18:  finalized_slot=12
Slot 24:  finalized_slot=18  ← finalized
Slot 30:  finalized_slot=18  ← STUCK
Slot 50:  finalized_slot=18  ← STILL STUCK
Slot 100: finalized_slot=18  ← NOT ADVANCING
```

## The 3SF-Mini Gap Rule

This is the most subtle cause of finalization stalls and is **protocol-level**,
not a client bug.

A slot N is "justifiable after finalized slot F" if `delta = N - F` is one of:
- ≤ 5 (any small distance)
- A perfect square: 9, 16, 25, 36, 49, 64, 81, 100, 121, 144, 169, 196, ...
- A pronic number `n*(n+1)`: 6, 12, 20, 30, 42, 56, 72, 90, 110, 132, 156, 182, ...

**Finalization rule:** Finalization advances from `source` to `target` only if
**NO slot between them is "justifiable but not justified."**

**Implication:** During normal steady-state operation, validators target the
*latest* justifiable slot at each step, so the chain of justified slots
matches the gap pattern and finalization keeps up. After a stall or peer
dropout, intermediate slots can be skipped — and once a justifiable slot is
missed, finalization can't cross it without retroactively justifying it
(which validators don't do).

**Symptom:** justification keeps advancing in big jumps (e.g., 100 → 145 →
196), but finalization is stuck far behind because some intermediate
justifiable slot was never directly justified.

This is **not a gean bug**. All clients exhibit this behavior because the
3SF-mini specification doesn't include a recovery mechanism. To recover,
restart from a clean genesis (or use checkpoint sync to bootstrap from a
known-good state).

## Common Causes of Finalization Stalls

### 1. Insufficient Validator Participation

**Requirement:** Need **>2/3 supermajority** to justify
- With 5 validators: need 4 votes (3*4=12 ≥ 2*5=10 ✓)
- With 6 validators: need 4 votes (3*4=12 ≥ 2*6=12 ✓)
- With 9 validators: need 6 votes (3*6=18 ≥ 2*9=18 ✓)

If validators are on different forks, neither fork may reach >2/3.

```bash
# Count how many validators are active (attesting)
grep "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u

# Check which validators are on which fork (by head block they attest to)
grep "head=0x" lantern_0.log | grep "validator=" | tail -30
```

### 2. Validators on Invalid Fork

If N validators follow an invalid fork, only (total - N) validators contribute
to canonical chain.

**Example:** 6 validators, 1 on invalid fork
- Total: 6 validators
- Honest: 5 validators on canonical fork
- Threshold: need 4 votes (3*4 ≥ 12)
- Available: 5 honest votes
- **Should justify!** 5 ≥ 4 ✓

**Example:** 6 validators, 3 on invalid fork
- Total: 6 validators
- Honest: 3 validators on canonical fork
- Threshold: need 4 votes
- Available: 3 honest votes
- **Cannot justify!** 3 < 4 ✗

### 3. Missing Attestations

Client fails to process attestations from certain validators.

```bash
# gean
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "attestation channel full"
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "Failed to process attestation"

# ethlambda
grep "Failed to process.*attestation" ethlambda_0.log | tail -30

# Common reasons:
# - "Unknown head block" → validator attesting to block this client doesn't have
# - "Unknown target block" → validator attesting to invalid/orphan fork blocks
```

**Impact:**
- Missing attestations reduce effective vote count
- May prevent reaching >2/3 threshold even if enough validators are on canonical fork

### 4. Justification Chain Broken (3SF-Mini Gap Rule)

3SF-mini requires justified slots at specific intervals (see top of this doc).
Missing blocks or attestations can break justification chain by skipping a
justifiable slot.

```bash
# Check justification progress (gean)
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "Latest Justified:\|justified_slot=" | tail -30

# Look for gaps in justified slots
```

### 5. Aggregator Crashed or Disconnected

In gean's network, the aggregator (one designated node, e.g., `gean_0` with
`--is-aggregator`) bundles individual signatures into aggregated proofs. If the
aggregator dies, raw attestations may not be aggregated, and other clients
won't see the supermajority needed to finalize.

```bash
# Check who is aggregating
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "\[signature\] aggregate:" | tail -20

# If empty, gean is not aggregating — was it started without --is-aggregator?
```

### 6. One or More Clients Crashed

A client crash mid-run reduces validator count, can drop below threshold.

```bash
# Sort logs by size — anomalously large logs are crash loops
wc -l *.log | sort -n

# Find panic patterns
grep -l "panic\|fatal\|stack overflow" *.log

# Check container status if devnet still running
docker ps --format "{{.Names}}: {{.Status}}" --filter "name=_0"
```

## Finalization Math

Given:
- `N` = total validators
- `N_honest` = validators on canonical fork
- `N_invalid` = validators on invalid/wrong fork
- Threshold = `3 * votes >= 2 * N` (i.e., `votes >= ceil(2N/3)`)

### Examples

**5 validators, all honest:**
- Total: 5 validators
- Threshold: `3v >= 10`, so `v >= 4`
- Available: 5
- **Justifies!** 5 ≥ 4 ✓

**5 validators, 1 crashed:**
- Total: 5 (registry size)
- Honest: 4
- Threshold: `v >= 4`
- Available: 4
- **Justifies!** 4 ≥ 4 ✓ (just enough)

**5 validators, 2 crashed:**
- Total: 5
- Honest: 3
- Threshold: `v >= 4`
- Available: 3
- **Cannot justify!** 3 < 4 ✗

**6 validators, 1 on invalid fork:**
- Total: 6
- Honest: 5
- Threshold: `v >= 4`
- Available: 5
- **Justifies!** 5 ≥ 4 ✓

**6 validators, 3 on invalid fork:**
- Total: 6
- Honest: 3
- Threshold: `v >= 4`
- Available: 3
- **Cannot justify!**

## Debugging Steps

### Step 1: Verify Validator Count and Status

```bash
# Count total validators
grep -h "validator=" *.log | grep -oE "validator=[0-9]+" | sort -u | wc -l

# Check which nodes are proposing blocks (active validators)
grep -h "We are the proposer\|proposing block\|proposed block" *.log | head -30

# Check which nodes are still alive (containers)
docker ps --format "{{.Names}}: {{.Status}}" --filter "name=_0"
```

### Step 2: Check Fork Structure

```bash
# See if clients have different heads
grep -h "Head Slot: 30\|head slot=30" *.log

# Compare block hashes at recent slots
for slot in 28 29 30 31 32; do
  echo "=== Slot $slot ==="
  sed 's/\x1b\[[0-9;]*m//g' *.log | grep -h "slot=$slot[^0-9]" | grep -oE "0x[a-f0-9]{8}" | sort -u
done
```

### Step 3: Count Attestations

```bash
# Count attestations received per slot (gean)
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "attestation verified.*slot=30" | wc -l

# Expected: N-1 attestations per slot (all validators except proposer)
# With 5 validators: expect 4 attestations per slot
```

### Step 4: Check for Processing Failures

```bash
# Look for attestation processing failures
grep "Failed to process.*attestation\|attestation.*fail" *.log | tail -50

# Group by error type
grep "Failed to process.*attestation" ethlambda_0.log | grep -oE "err=.*" | sort | uniq -c
```

### Step 5: Verify Threshold Calculation

```bash
# Calculate if finalization should be possible
echo "Total validators: $(grep -h validator= *.log | grep -oE 'validator=[0-9]+' | sort -u | wc -l)"
echo "Threshold: 3*votes >= 2*total"
echo "Validators on canonical fork: ?"  # Count from logs
```

### Step 6: Check the 3SF-Mini Gap Rule

If `justified` is advancing but `finalized` isn't, you're likely hitting the
3SF-mini gap rule. Check whether intermediate justifiable slots have been
skipped:

```bash
# Print all justified slots gean has seen
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "justified_slot=" | grep -oE "justified_slot=[0-9]+" | sort -u

# Compute the expected justifiable slots from finalized:
# delta values: 1,2,3,4,5,6,9,12,16,20,25,30,36,42,49,56,64,72,81,90,100,...
# If finalized=64, expected justifiable = 65,66,67,68,69,70,73,76,80,84,89,94,100,...
```

## Known Bugs (Resolved)

### gean: Block Bloat (FIXED in commit 62454aa)

**Old symptom:** gean produced blocks with 100+ aggregated attestations,
exceeding the 10 MiB spec limit. Blocks failed to gossip, network stalled.

**Fix:** Per-validator latest-vote selection in `node/store_build.go`. After
the fix, gean's blocks contain at most `numValidators` distinct attestations.

**Detection (regression check):**
```bash
# Should NEVER print after the fix
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep -E "attestations=[0-9]{2,}" | head
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep "MessageTooLarge\|exceeds max"
```

### ethlambda: Same Bug (UPSTREAM ISSUE)

ethlambda still has the same block bloat bug at the time of writing
(`crates/blockchain/src/store.rs:1018`). When ethlambda is the proposer in a
mixed network, expect occasional MessageTooLarge / cascade behavior.

## Additional Resources

See [FORK_ANALYSIS.md](FORK_ANALYSIS.md) for fork detection and
[ERROR_CLASSIFICATION.md](ERROR_CLASSIFICATION.md) for common error patterns.
