# Finalization Debugging

Guide to debugging finalization issues during devnet runs.

## What is Finalization?

Finalization is the process of making blocks permanent and irreversible. In lean consensus:
- Blocks can be reverted before finalization
- After finalization, blocks cannot be changed
- Finalization requires >2/3 supermajority of validator votes

## Finalization Requirements

### Threshold Math
| Validators | >2/3 Required | Minimum Votes |
|------------|---------------|---------------|
| 6 | 4.67 | 5 |
| 9 | 6.0 | 7 |
| 12 | 8.0 | 9 |
| 18 | 12.0 | 13 |

### Finalization Frequency
- Finalization should occur every 6-12 slots
- If finalization stops, consensus is broken

## Checking Finalization Progress

### gean
```bash
# Check finalization progress
grep "finalized_slot=" gean_0.log | tail -20

# Check if finalization is stuck
grep "finalized_slot=" gean_0.log | awk -F'finalized_slot=' '{print $2}' | awk '{print $1}' | uniq -c
```

### Other Clients
```bash
# ethlambda
grep "Finalized block" ethlambda_0.log | tail -20

# qlean
grep "Finalized block:" qlean_0.log | tail -20

# lantern
grep "finalized" lantern_0.log | tail -20
```

## Common Finalization Stalls

### 1. Validator Participation Low
If <2/3 of validators are attesting, finalization cannot occur.

**Symptoms:**
- justified_slot not advancing
- High "skipping validator duties" count

**Debug:**
```bash
# Check attestations per slot
grep "attestations=" gean_0.log | tail -50
```

### 2. Fork Blocking Finalization
If clients are on different forks, neither can achieve 2/3 majority.

**Symptoms:**
- Different head_slot across clients
- "rejected gossip block" errors

**Debug:** See FORK_ANALYSIS.md

### 3. State Not Propagating
If new states aren't propagating, attestations to new blocks fail.

**Symptoms:**
- "parent state missing" warnings
- Sync catching up repeatedly

**Debug:**
```bash
grep "parent state missing" gean_0.log | head -20
```

## Justification Chain Analysis

Finalization requires a valid justification chain:

1. **Previous checkpoint justified** → Current checkpoint can be justified
2. **2/3 attestations to checkpoint** → Checkpoint becomes justified
3. **Previous epoch justified** → Current epoch can be finalized

```bash
# Check justification progression
grep -E "justified_slot=|justified_root=" gean_0.log | tail -20
```

## Step-by-Step Debugging

1. **Check if finalization is actually stuck**
   ```bash
   # Compare first and last finalized_slot
   grep "finalized_slot=" gean_0.log | head -1
   grep "finalized_slot=" gean_0.log | tail -1
   ```

2. **Check validator participation**
   ```bash
   # Count attestations per slot
   grep "slot=10" gean_0.log | grep attestation
   ```

3. **Check for forks**
   ```bash
   # Compare head across clients
   grep "head_slot=" gean_0.log | tail -5
   grep "Head Slot:" ethlambda_0.log | tail -5
   ```

4. **Check peer connectivity**
   ```bash
   # Are all nodes connected?
   grep "peer connected" gean_0.log | wc -l
   grep "peer connected" ethlambda_0.log | wc -l
   ```

## Finalization Healthy Indicators

- finalized_slot advancing every 6-12 slots
- justified_slot advancing ahead of head_slot
- Few or no "behind peers" warnings
- All clients have similar head_slot

## Finalization Problem Indicators

- finalized_slot same for 50+ slots
- Large gap between justified_slot and finalized_slot
- High count of "skipping validator duties"
- Different finalization slots across clients
