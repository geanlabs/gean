# Memory Leak Analysis: Chain-Following Operation

## Problem

Gean exhibits steady memory growth while actively following the chain under normal operation — not limited to sync or catchup. The node's heap grows indefinitely because no data is ever pruned after finalization advances.

## Root Cause

Gean has **zero finalization-based pruning**. The leanSpec reference specification (`store.py:228-268`) calls `prune_stale_attestation_data()` every time finalization advances. ethlambda implements bounded circular buffers plus multi-level pruning on finalization. Lantern prunes fork choice states on finalization. Gean does none of this.

Every block processed adds entries to multiple maps and storage backends. Nothing is ever removed.

## Leak Sources

### 1. Unbounded block and state storage (CRITICAL)

**Files:** `storage/memory/memory.go`, `storage/bolt/bolt.go`

The `storage.Store` interface defines `PutBlock`, `PutState`, and `PutSignedBlock` but has no delete operations. Every processed block and its post-state are stored permanently.

At one block per 4-second slot, this accumulates ~21,600 blocks and ~21,600 states per day. States are the heaviest objects (validators list, historical block hashes, justification bitfields).

### 2. Unbounded storage.GetAllBlocks() in fork choice (HIGH)

**File:** `chain/forkchoice/store.go:284-296`

```go
func (c *Store) allKnownBlockSummaries() map[[32]byte]blockSummary {
    blocks := c.storage.GetAllBlocks()
    summaries := make(map[[32]byte]blockSummary, len(blocks)+len(c.checkpointRoots))
    ...
}
```

Called every head update (`updateHeadLocked`, line 99) and safe target update (`updateSafeTargetLocked`, line 197). Copies **every block from storage** into a new map each time. As storage grows unbounded (leak #1), this allocation grows proportionally, creating GC pressure and O(n) cost per slot where n is total blocks ever stored.

Note: `checkpointRoots` itself is static — only populated at store creation from `HistoricalBlockHashes` (lines 153, 225) and never written to during operation. It is not a leak source.

### 3. latestKnownAggregatedPayloads never pruned (HIGH)

**File:** `chain/forkchoice/time.go:84`

```go
c.latestKnownAggregatedPayloads = mergeAggregatedPayloads(
    c.latestKnownAggregatedPayloads, c.latestNewAggregatedPayloads)
```

`mergeAggregatedPayloads` only adds entries. Aggregated payloads from finalized slots accumulate indefinitely. The leanSpec reference prunes these when `target.slot <= finalized_slot`.

### 4. aggregatedPayloads proof cache — keys grow unbounded (HIGH)

**File:** `chain/forkchoice/store.go:41`, `chain/forkchoice/signature_cache.go:69-103`

```go
aggregatedPayloads map[signatureKey][]storedAggregatedPayload
```

Used for proof reuse in `findReusableAggregatedProof()`. Each key (`{validatorID, dataRoot}`) is capped at `maxProofsPerKey = 8` entries (signature_cache.go:98-101), so per-key growth is bounded. However, the **number of distinct keys** grows unbounded — one new key per unique `{validatorID, attestationData}` combination. Keys for long-finalized attestation data are never removed.

### 5. gossipSignatures stored by non-aggregator nodes (MEDIUM)

**File:** `chain/forkchoice/attestation.go:141`

```go
// Inside processAttestationLocked, isFromBlock == false path:
c.storeGossipSignatureLocked(sa)
```

The public `ProcessAttestation()` method (line 22-24) guards `storeGossipSignatureLocked` behind `isAggregator`. But `processAttestationLocked` at line 141 calls it **unconditionally** for all non-block attestations. This means non-aggregator nodes accumulate gossip signatures they will never use for aggregation. For aggregator nodes, the map is cleared per aggregation cycle (`aggregation.go:61`), but between cycles it also accumulates entries for finalized slots.

### 6. XMSS keypairs never freed (LOW)

**File:** `node/lifecycle.go:300-304`

`Keypair.Free()` exists in `xmss/leansig/leansig.go:87` and is correctly used in tests and the keygen tool (`cmd/keygen/main.go:36`), but is never called in the node lifecycle. Keypairs loaded at `lifecycle.go:300` via `leansig.LoadKeypair()` are stored in the validator map for the node's lifetime. Rust-allocated memory is never returned on shutdown. This is a fixed-size leak (not growing), but still unreturned memory.

## Reference: How Other Clients Handle This

### leanSpec (reference specification)

`store.py:228-268` defines `prune_stale_attestation_data()`:
- Removes entries from `attestation_signatures`, `latest_new_aggregated_payloads`, and `latest_known_aggregated_payloads` where `target.slot <= finalized_slot`
- Called in two places: after `on_block()` (line 559) and after building aggregated payloads (line 1312)
- Trigger condition: `store.latest_finalized.slot > self.latest_finalized.slot`

### ethlambda (best-in-class pruning)

Two-tier defense:

1. **Bounded circular buffers** (`store.rs:96-175`): `PayloadBuffer` with `NEW_PAYLOAD_CAP = 512` caps memory even when finalization stalls.
2. **Finalization-triggered pruning**: `prune_live_chain()`, `prune_gossip_signatures()`, `prune_attestation_data_by_root()`, `prune_old_data()` — each targets a specific data structure.
3. **Retention limits**: ~21,600 blocks (~1 day), ~3,000 states (~3.3 hours).

### lantern

`fork_choice.c:1356-1416`: `lantern_fork_choice_prune_states()` walks from head to finalized block, marks canonical path, deletes off-chain states. Called after finalization advances.

## Proposed Fix

### Step 1: Add delete operations to the storage interface

**File:** `storage/interface.go`

Add `DeleteBlock`, `DeleteSignedBlock`, and `DeleteState` methods. Implement in both `storage/memory/memory.go` (delete from map) and `storage/bolt/bolt.go` (delete from bucket).

### Step 2: Implement finalization-triggered pruning in the fork choice store

**File:** `chain/forkchoice/block.go`

In `ProcessBlock()`, after detecting finalization advancement (lines 171-175), call a new `pruneOnFinalization()` method.

The pruning method will:

1. **Prune attestation data** — remove entries from `latestKnownAggregatedPayloads` and `latestNewAggregatedPayloads` where the attestation data's `Target.Slot <= finalizedSlot`. This directly matches leanSpec's `prune_stale_attestation_data()` (store.py:245-268).
2. **Prune aggregatedPayloads cache** — remove keys from `aggregatedPayloads` where the associated slot is at or below finalized. The slot is stored in `storedAggregatedPayload.slot` (signature_cache.go:22).
3. **Prune storage** — delete blocks, signed blocks, and states below the finalized slot that are not on the canonical chain. Keep a retention buffer (e.g., 64 slots) beyond finalized for reorg safety. Walk from finalized root backward to identify the canonical chain; delete everything else.

### Step 3: Guard gossipSignatures behind isAggregator in processAttestationLocked

**File:** `chain/forkchoice/attestation.go:141`

Wrap the `storeGossipSignatureLocked(sa)` call at line 141 with `if c.isAggregator { ... }` to prevent non-aggregator nodes from accumulating unused gossip signatures. This matches the guard already present in the public `ProcessAttestation()` (line 22-24) and `processSubnetAttestationLocked` (line 76-78).

### Step 4: Free XMSS keypairs on shutdown

**File:** `node/lifecycle.go`

Call `kp.Free()` on all loaded keypairs during node shutdown or context cancellation.

### Step 5: Add bounded payload buffers (defense against stalled finalization)

**File:** `chain/forkchoice/store.go`

Introduce a maximum capacity for `latestKnownAggregatedPayloads`. When capacity is reached, evict the oldest entry (by slot). This ensures memory stays bounded even if finalization stalls for an extended period — matching ethlambda's `PayloadBuffer` pattern (store.rs:102-112, cap=512).

## Verification

1. Run `make unit-test` and `make spec-test` to confirm pruning does not break consensus.
2. Run `make test-race` to verify pruning under mutex is free of data races.
3. Add a unit test that processes N blocks, advances finalization, and asserts that storage size and map sizes decrease.
4. Profile heap growth over 1000+ slots with and without the fix using `GODEBUG=memprofrate=1`.
