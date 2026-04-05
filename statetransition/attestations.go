package statetransition

import (
	"sort"

	"github.com/geanlabs/gean/types"
)

// ProcessAttestations processes all aggregated attestations in a block body,
// updating justification and finalization state.
//
func ProcessAttestations(state *types.State, attestations []*types.AggregatedAttestation) error {
	validatorCount := int(state.NumValidators())
	if validatorCount == 0 {
		return nil
	}

	// Precondition: justifications_roots must not contain zero hashes (spec state.py L389).
	for _, root := range state.JustificationsRoots {
		var r [32]byte
		copy(r[:], root)
		if types.IsZeroRoot(r) {
			return ErrZeroHashInJustificationRoots
		}
	}

	// Reconstruct pending justifications from flat SSZ storage into a map.
	// Key: target root, Value: per-validator vote booleans.
	justifications := reconstructJustifications(state, validatorCount)

	// Build root → slot lookup for finalization pruning.
	rootToSlot := buildRootToSlot(state)

	for _, agg := range attestations {
		source := agg.Data.Source
		target := agg.Data.Target

		if !isValidVote(state, source, target) {
			continue
		}

		// Get or create vote tracking for this target root.
		votes, exists := justifications[target.Root]
		if !exists {
			votes = make([]bool, validatorCount)
			justifications[target.Root] = votes
		}

		// Reject oversized aggregation_bits (spec would crash on OOB).
		bitsLen := types.BitlistLen(agg.AggregationBits)
		if bitsLen > uint64(validatorCount) {
			continue
		}

		// Mark validators as having voted.
		for i := uint64(0); i < bitsLen; i++ {
			if types.BitlistGet(agg.AggregationBits, i) {
				votes[i] = true
			}
		}

		// Check supermajority: 3 * votes >= 2 * validators.
		voteCount := countTrue(votes)
		if 3*voteCount >= 2*validatorCount {
			// Justify the target.
			state.LatestJustified = target
			setSlotJustified(state, state.LatestFinalized.Slot, target.Slot)

			// Remove from pending (now justified).
			delete(justifications, target.Root)

			// Try to finalize source.
			tryFinalize(state, source, target, &justifications, rootToSlot)
		}
	}

	// Serialize back to flat SSZ storage (sorted for determinism).
	serializeJustifications(state, justifications, validatorCount)

	return nil
}

// isValidVote checks the 6 validation rules for an attestation vote.
func isValidVote(state *types.State, source, target *types.Checkpoint) bool {
	finalizedSlot := state.LatestFinalized.Slot

	// 1. Source must already be justified.
	if !isSlotJustified(state, finalizedSlot, source.Slot) {
		return false
	}
	// 2. Target must not already be justified.
	if isSlotJustified(state, finalizedSlot, target.Slot) {
		return false
	}
	// 3. Neither root can be zero.
	if types.IsZeroRoot(source.Root) || types.IsZeroRoot(target.Root) {
		return false
	}
	// 4. Both checkpoints must exist in historical_block_hashes.
	if !checkpointExists(state, source) || !checkpointExists(state, target) {
		return false
	}
	// 5. Time flows forward.
	if target.Slot <= source.Slot {
		return false
	}
	// 6. Target is justifiable after finalized (3SF-mini).
	if !SlotIsJustifiableAfter(target.Slot, finalizedSlot) {
		return false
	}
	return true
}

// tryFinalize attempts to advance finalization from source to target.
// Finalization succeeds when there are no justifiable slots between
// source.slot and target.slot (exclusive).
func tryFinalize(
	state *types.State,
	source, target *types.Checkpoint,
	justifications *map[[32]byte][]bool,
	rootToSlot map[[32]byte]uint64,
) {
	// Check for any justifiable slot in the gap.
	for s := source.Slot + 1; s < target.Slot; s++ {
		if SlotIsJustifiableAfter(s, state.LatestFinalized.Slot) {
			return // gap exists, cannot finalize
		}
	}

	oldFinalizedSlot := state.LatestFinalized.Slot
	state.LatestFinalized = source

	// Shift justified_slots window forward.
	delta := state.LatestFinalized.Slot - oldFinalizedSlot
	shiftJustifiedSlots(state, delta)

	// Prune justifications whose roots are at or below the new finalized slot.
	for root := range *justifications {
		slot, found := rootToSlot[root]
		if !found || slot <= state.LatestFinalized.Slot {
			delete(*justifications, root)
		}
	}
}

// --- justified_slots operations ---

// isSlotJustified checks if a slot is justified.
// Slots at or before finalized are implicitly justified.
func isSlotJustified(state *types.State, finalizedSlot, slot uint64) bool {
	if slot <= finalizedSlot {
		return true
	}
	relIndex := slot - finalizedSlot - 1
	return types.BitlistGet(state.JustifiedSlots, relIndex)
}

// setSlotJustified marks a slot as justified.
func setSlotJustified(state *types.State, finalizedSlot, slot uint64) {
	if slot <= finalizedSlot {
		return
	}
	relIndex := slot - finalizedSlot - 1
	jsLen := types.BitlistLen(state.JustifiedSlots)
	if relIndex >= jsLen {
		state.JustifiedSlots = types.BitlistExtend(state.JustifiedSlots, relIndex+1)
	}
	types.BitlistSet(state.JustifiedSlots, relIndex)
}

// shiftJustifiedSlots drops `delta` bits from the front when finalization advances.
func shiftJustifiedSlots(state *types.State, delta uint64) {
	if delta == 0 {
		return
	}
	oldLen := types.BitlistLen(state.JustifiedSlots)
	if delta >= oldLen {
		state.JustifiedSlots = types.NewBitlistSSZ(0)
		return
	}
	newLen := oldLen - delta
	newBits := types.NewBitlistSSZ(newLen)
	for i := uint64(0); i < newLen; i++ {
		if types.BitlistGet(state.JustifiedSlots, i+delta) {
			types.BitlistSet(newBits, i)
		}
	}
	state.JustifiedSlots = newBits
}

// --- helpers ---

func checkpointExists(state *types.State, cp *types.Checkpoint) bool {
	slot := cp.Slot
	if slot >= uint64(len(state.HistoricalBlockHashes)) {
		return false
	}
	var stored [32]byte
	copy(stored[:], state.HistoricalBlockHashes[slot])
	return stored == cp.Root
}

func countTrue(votes []bool) int {
	count := 0
	for _, v := range votes {
		if v {
			count++
		}
	}
	return count
}

// reconstructJustifications converts flat SSZ storage into a vote map.
func reconstructJustifications(state *types.State, validatorCount int) map[[32]byte][]bool {
	justifications := make(map[[32]byte][]bool)
	for i, rootBytes := range state.JustificationsRoots {
		var root [32]byte
		copy(root[:], rootBytes)
		votes := make([]bool, validatorCount)
		for v := 0; v < validatorCount; v++ {
			bitIdx := uint64(i*validatorCount + v)
			if types.BitlistGet(state.JustificationsValidators, bitIdx) {
				votes[v] = true
			}
		}
		justifications[root] = votes
	}
	return justifications
}

// buildRootToSlot maps each root to its latest slot in historical_block_hashes.
func buildRootToSlot(state *types.State) map[[32]byte]uint64 {
	rootToSlot := make(map[[32]byte]uint64)
	start := state.LatestFinalized.Slot + 1
	for i := start; i < uint64(len(state.HistoricalBlockHashes)); i++ {
		var root [32]byte
		copy(root[:], state.HistoricalBlockHashes[i])
		if existing, ok := rootToSlot[root]; !ok || i > existing {
			rootToSlot[root] = i
		}
	}
	return rootToSlot
}

// serializeJustifications converts vote map back to flat SSZ storage.
// Roots are sorted for deterministic output.
func serializeJustifications(state *types.State, justifications map[[32]byte][]bool, validatorCount int) {
	// Sort roots for deterministic output.
	roots := make([][32]byte, 0, len(justifications))
	for root := range justifications {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		for k := 0; k < 32; k++ {
			if roots[i][k] != roots[j][k] {
				return roots[i][k] < roots[j][k]
			}
		}
		return false
	})

	// Rebuild justifications_roots.
	sszRoots := make([][]byte, len(roots))
	for i, root := range roots {
		r := make([]byte, 32)
		copy(r, root[:])
		sszRoots[i] = r
	}
	state.JustificationsRoots = sszRoots

	// Rebuild justifications_validators.
	totalBits := uint64(len(roots)) * uint64(validatorCount)
	if totalBits == 0 {
		state.JustificationsValidators = types.NewBitlistSSZ(0)
		return
	}
	bits := types.NewBitlistSSZ(totalBits)
	for i, root := range roots {
		votes := justifications[root]
		for v := 0; v < validatorCount; v++ {
			if votes[v] {
				types.BitlistSet(bits, uint64(i*validatorCount+v))
			}
		}
	}
	state.JustificationsValidators = bits
}
