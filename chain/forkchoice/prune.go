package forkchoice

import (
	"github.com/geanlabs/gean/types"
)

// Storage retention limits aligned with ethlambda (store.rs:83-92).
const (
	// blocksToKeep is ~1 day of block history at 4-second slots (86400/4).
	blocksToKeep = 21_600

	// statesToKeep is ~3.3 hours of state history at 4-second slots (12000/4).
	statesToKeep = 3_000

	// maxKnownAggregatedPayloads caps the known aggregated payloads map to
	// prevent unbounded growth during stalled finalization. Matches
	// ethlambda's AGGREGATED_PAYLOAD_CAP.
	maxKnownAggregatedPayloads = 4096

	// maxAggregatedPayloadKeys caps the aggregatedPayloads proof cache to
	// prevent unbounded key growth.
	maxAggregatedPayloadKeys = 8192
)

// pruneOnFinalization removes data that can no longer influence fork choice
// after finalization advances. Matches leanSpec prune_stale_attestation_data()
// (store.py:228-268).
//
// Must be called with c.mu held.
func (c *Store) pruneOnFinalization() {
	finalizedSlot := c.latestFinalized.Slot

	c.pruneStaleAttestationData(finalizedSlot)
	c.pruneAggregatedPayloadsCache(finalizedSlot)
	c.pruneStorage(finalizedSlot)
}

// pruneStaleAttestationData removes aggregated payload entries where the
// attestation target slot is at or before the finalized slot. Matches
// leanSpec store.py:245-268 which filters by target.slot > finalized_slot.
func (c *Store) pruneStaleAttestationData(finalizedSlot uint64) {
	for key, payload := range c.latestKnownAggregatedPayloads {
		if payload.data != nil && payload.data.Target != nil && payload.data.Target.Slot <= finalizedSlot {
			delete(c.latestKnownAggregatedPayloads, key)
		}
	}
	for key, payload := range c.latestNewAggregatedPayloads {
		if payload.data != nil && payload.data.Target != nil && payload.data.Target.Slot <= finalizedSlot {
			delete(c.latestNewAggregatedPayloads, key)
		}
	}
}

// pruneAggregatedPayloadsCache removes signature cache entries for
// attestation data at or before the finalized slot.
func (c *Store) pruneAggregatedPayloadsCache(finalizedSlot uint64) {
	for key, entries := range c.aggregatedPayloads {
		if len(entries) > 0 && entries[0].slot <= finalizedSlot {
			delete(c.aggregatedPayloads, key)
		}
	}
	for key, stored := range c.gossipSignatures {
		if stored.slot <= finalizedSlot {
			delete(c.gossipSignatures, key)
		}
	}
}

// pruneStorage removes blocks and states that are below the finalized slot
// and not on the canonical chain. Uses retention limits for blocks and states.
//
// Uses ForEachBlock to iterate without copying the full block map.
func (c *Store) pruneStorage(finalizedSlot uint64) {
	if finalizedSlot == 0 {
		return
	}

	// Collect canonical chain roots by walking from head to finalized root.
	canonical := make(map[[32]byte]struct{})
	current := c.head
	for {
		canonical[current] = struct{}{}
		block, ok := c.storage.GetBlock(current)
		if !ok {
			break
		}
		if block.Slot <= finalizedSlot {
			break
		}
		current = block.ParentRoot
	}
	// Always keep finalized and justified roots.
	canonical[c.latestFinalized.Root] = struct{}{}
	canonical[c.latestJustified.Root] = struct{}{}

	// State retention: prune canonical states older than this threshold.
	// Matches ethlambda STATES_TO_KEEP (~3.3 hours).
	var pruneStatesBelow uint64
	if finalizedSlot > statesToKeep {
		pruneStatesBelow = finalizedSlot - statesToKeep
	}

	// Single pass: collect roots to delete. We cannot delete during
	// ForEachBlock iteration (bolt doesn't allow mutation during View tx).
	var deleteRoots [][32]byte
	var deleteStateOnlyRoots [][32]byte

	c.storage.ForEachBlock(func(root [32]byte, block *types.Block) bool {
		if block.Slot >= finalizedSlot {
			return true // keep: at or above finalized
		}

		if _, ok := canonical[root]; !ok {
			// Non-canonical block below finalized: delete everything.
			deleteRoots = append(deleteRoots, root)
			return true
		}

		// Canonical block below finalized: keep block, but prune old states.
		if pruneStatesBelow > 0 && block.Slot < pruneStatesBelow {
			if root != c.latestFinalized.Root && root != c.latestJustified.Root {
				deleteStateOnlyRoots = append(deleteStateOnlyRoots, root)
			}
		}
		return true
	})

	for _, root := range deleteRoots {
		c.storage.DeleteBlock(root)
		c.storage.DeleteSignedBlock(root)
		c.storage.DeleteState(root)
	}
	for _, root := range deleteStateOnlyRoots {
		c.storage.DeleteState(root)
	}

	if len(deleteRoots) > 0 || len(deleteStateOnlyRoots) > 0 {
		log.Info("pruned storage on finalization",
			"finalized_slot", finalizedSlot,
			"blocks_deleted", len(deleteRoots),
			"states_deleted", len(deleteRoots)+len(deleteStateOnlyRoots),
		)
	}
}

// enforcePayloadCap evicts the oldest entries from latestKnownAggregatedPayloads
// when the map exceeds maxKnownAggregatedPayloads. This bounds memory even when
// finalization stalls. Matches ethlambda's FIFO PayloadBuffer pattern.
func (c *Store) enforcePayloadCap() {
	for len(c.latestKnownAggregatedPayloads) > maxKnownAggregatedPayloads {
		var oldestKey [32]byte
		oldestSlot := uint64(^uint64(0))
		found := false
		for key, payload := range c.latestKnownAggregatedPayloads {
			if payload.data != nil && payload.data.Target != nil && payload.data.Target.Slot < oldestSlot {
				oldestSlot = payload.data.Target.Slot
				oldestKey = key
				found = true
			}
		}
		if !found {
			break
		}
		delete(c.latestKnownAggregatedPayloads, oldestKey)
	}
}

// enforceAggregatedPayloadsCacheCap bounds the aggregatedPayloads proof cache
// keys to prevent unbounded growth independent of finalization.
func (c *Store) enforceAggregatedPayloadsCacheCap() {
	for len(c.aggregatedPayloads) > maxAggregatedPayloadKeys {
		var oldestKey signatureKey
		oldestSlot := uint64(^uint64(0))
		found := false
		for key, entries := range c.aggregatedPayloads {
			if len(entries) > 0 && entries[0].slot < oldestSlot {
				oldestSlot = entries[0].slot
				oldestKey = key
				found = true
			}
		}
		if !found {
			break
		}
		delete(c.aggregatedPayloads, oldestKey)
	}
}
