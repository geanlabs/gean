package statetransition

import (
	"bytes"
	"sort"

	"github.com/geanlabs/gean/types"
)

// ProcessAttestations applies attestation votes and updates
// justification/finalization according to leanSpec 3SF-mini rules.
//
// Per-validator votes are tracked via justifications_roots (sorted list of
// block roots being voted on) and justifications_validators (flat bitlist
// where each root's validator votes are packed consecutively).
func ProcessAttestations(state *types.State, attestations []*types.AggregatedAttestation) *types.State {
	numValidators := uint64(len(state.Validators))

	// Deserialize justifications from SSZ form into working map.
	justifications := make(map[[32]byte][]bool)
	for i, root := range state.JustificationsRoots {
		votes := make([]bool, numValidators)
		for v := uint64(0); v < numValidators; v++ {
			bitIdx := uint64(i)*numValidators + v
			votes[v] = GetBit(state.JustificationsValidators, bitIdx)
		}
		justifications[root] = votes
	}

	justifiedSlots := CloneBitlist(state.JustifiedSlots)
	latestJustified := &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot}
	latestFinalized := &types.Checkpoint{Root: state.LatestFinalized.Root, Slot: state.LatestFinalized.Slot}
	finalizedSlot := latestFinalized.Slot
	originalFinalizedSlot := state.LatestFinalized.Slot

	// Map each known root to its latest materialized slot after the finalized boundary.
	rootToSlot := make(map[[32]byte]uint64)
	startSlot := finalizedSlot + 1
	for i := startSlot; i < uint64(len(state.HistoricalBlockHashes)); i++ {
		root := state.HistoricalBlockHashes[i]
		if prev, ok := rootToSlot[root]; !ok || i > prev {
			rootToSlot[root] = i
		}
	}

	processVote := func(validatorID uint64, data *types.AttestationData) {
		if data == nil || data.Source == nil || data.Target == nil {
			return
		}

		source := data.Source
		target := data.Target
		srcSlot := source.Slot
		tgtSlot := target.Slot

		// Target must be after source (strict).
		if tgtSlot <= srcSlot {
			return
		}

		// Source must be justified. Slots at/before finalized are implicitly justified.
		if !isSlotJustified(justifiedSlots, finalizedSlot, srcSlot) {
			return
		}

		// Target must not already be justified.
		if isSlotJustified(justifiedSlots, finalizedSlot, tgtSlot) {
			return
		}

		// Source root must match historical block hashes.
		if srcSlot >= uint64(len(state.HistoricalBlockHashes)) || state.HistoricalBlockHashes[srcSlot] != source.Root {
			return
		}

		// Target root must match historical block hashes.
		if tgtSlot >= uint64(len(state.HistoricalBlockHashes)) || state.HistoricalBlockHashes[tgtSlot] != target.Root {
			return
		}

		// Target must be justifiable after the original finalized slot.
		if !types.IsJustifiableAfter(tgtSlot, originalFinalizedSlot) {
			return
		}

		// Validate validator ID.
		if validatorID >= numValidators {
			return
		}

		// Record vote (idempotent — skip if already voted).
		if _, ok := justifications[target.Root]; !ok {
			justifications[target.Root] = make([]bool, numValidators)
		}
		if justifications[target.Root][validatorID] {
			return
		}
		justifications[target.Root][validatorID] = true

		// Count votes for this target.
		count := uint64(0)
		for _, voted := range justifications[target.Root] {
			if voted {
				count++
			}
		}

		// Supermajority: 3 * count >= 2 * numValidators.
		if 3*count < 2*numValidators {
			return
		}

		// Justify target.
		latestJustified = &types.Checkpoint{Root: target.Root, Slot: tgtSlot}
		justifiedSlots = extendJustifiedSlotsToSlot(justifiedSlots, finalizedSlot, tgtSlot)
		justifiedSlots = setSlotJustified(justifiedSlots, finalizedSlot, tgtSlot, true)
		delete(justifications, target.Root)

		// Finalization: if no justifiable slot exists between source and target,
		// then source becomes finalized.
		hasJustifiableGap := false
		for s := srcSlot + 1; s < tgtSlot; s++ {
			if types.IsJustifiableAfter(s, originalFinalizedSlot) {
				hasJustifiableGap = true
				break
			}
		}
		if !hasJustifiableGap {
			oldFinalizedSlot := finalizedSlot
			latestFinalized = &types.Checkpoint{Root: source.Root, Slot: srcSlot}
			finalizedSlot = latestFinalized.Slot

			// Rebase the justified-slots tracking window and prune stale pending votes.
			if finalizedSlot > oldFinalizedSlot {
				justifiedSlots = shiftJustifiedSlotsWindow(justifiedSlots, finalizedSlot-oldFinalizedSlot)
				for root := range justifications {
					slot, ok := rootToSlot[root]
					if !ok || slot <= finalizedSlot {
						delete(justifications, root)
					}
				}
			}
		}
	}

	for _, aggregated := range attestations {
		if aggregated == nil || aggregated.Data == nil {
			continue
		}
		numBits := uint64(BitlistLen(aggregated.AggregationBits))
		for validatorID := uint64(0); validatorID < numBits; validatorID++ {
			if !GetBit(aggregated.AggregationBits, validatorID) {
				continue
			}
			processVote(validatorID, aggregated.Data)
		}
	}

	// Serialize justifications back to SSZ form.
	sortedRoots := sortedJustificationRoots(justifications)
	flatVotes := flattenVotes(sortedRoots, justifications, numValidators)

	out := state.Copy()
	out.JustifiedSlots = justifiedSlots
	out.LatestJustified = latestJustified
	out.LatestFinalized = latestFinalized
	out.JustificationsRoots = sortedRoots
	out.JustificationsValidators = flatVotes
	return out
}

// sortedJustificationRoots returns the roots in deterministic (lexicographic) order.
func sortedJustificationRoots(justifications map[[32]byte][]bool) [][32]byte {
	roots := make([][32]byte, 0, len(justifications))
	for root := range justifications {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return bytes.Compare(roots[i][:], roots[j][:]) < 0
	})
	return roots
}

// flattenVotes serializes per-root validator votes into a single SSZ bitlist.
// For each root (in sortedRoots order), numValidators bits are appended.
func flattenVotes(sortedRoots [][32]byte, justifications map[[32]byte][]bool, numValidators uint64) []byte {
	totalBits := uint64(len(sortedRoots)) * numValidators
	if totalBits == 0 {
		return []byte{0x01} // empty bitlist with sentinel
	}

	numBytes := (totalBits + 1 + 7) / 8 // +1 for sentinel
	bl := make([]byte, numBytes)

	bitPos := uint64(0)
	for _, root := range sortedRoots {
		votes := justifications[root]
		for _, voted := range votes {
			if voted {
				bl[bitPos/8] |= 1 << (bitPos % 8)
			}
			bitPos++
		}
	}

	// Set sentinel bit at position totalBits.
	bl[totalBits/8] |= 1 << (totalBits % 8)

	return bl
}
