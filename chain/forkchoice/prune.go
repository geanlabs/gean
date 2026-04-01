package forkchoice

import (
	"github.com/geanlabs/gean/types"
)

// pruneOnFinalization removes data that can no longer influence fork choice
// after finalization advances. Matches leanSpec prune_stale_attestation_data().
//
// Must be called with c.mu held.
func (c *Store) pruneOnFinalization() {
	finalizedSlot := c.latestFinalized.Slot

	c.pruneStaleAttestationData(finalizedSlot)
	c.pruneAggregatedPayloadsCache(finalizedSlot)
	c.pruneStorage(finalizedSlot)
}

// pruneStaleAttestationData removes aggregated payload entries where the
// attestation target slot is at or before the finalized slot. This matches
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
// and not on the canonical chain. Keeps a small buffer for reorg safety.
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
	// Always keep finalized root.
	canonical[c.latestFinalized.Root] = struct{}{}
	// Keep justified root if different.
	canonical[c.latestJustified.Root] = struct{}{}

	// State retention: prune canonical states older than this threshold.
	const stateRetentionBuffer = 64
	var pruneStatesBelow uint64
	if finalizedSlot > stateRetentionBuffer {
		pruneStatesBelow = finalizedSlot - stateRetentionBuffer
	}

	// Single pass: collect roots to delete. We cannot delete during
	// ForEachBlock iteration (bolt doesn't allow mutation during View tx),
	// so we collect first, then delete.
	var deleteRoots [][32]byte
	var deleteStateOnlyRoots [][32]byte

	c.storage.ForEachBlock(func(root [32]byte, block *types.Block) bool {
		if block.Slot >= finalizedSlot {
			return true // keep: at or above finalized
		}

		isCanonical := false
		if _, ok := canonical[root]; ok {
			isCanonical = true
		}

		if !isCanonical {
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
}

// maxKnownAggregatedPayloads caps the known aggregated payloads map to prevent
// unbounded growth during stalled finalization. Evicts oldest entries by slot.
const maxKnownAggregatedPayloads = 512

// enforcePayloadCap evicts the oldest entries from latestKnownAggregatedPayloads
// when the map exceeds maxKnownAggregatedPayloads.
func (c *Store) enforcePayloadCap() {
	for len(c.latestKnownAggregatedPayloads) > maxKnownAggregatedPayloads {
		// Find the entry with the lowest target slot.
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
