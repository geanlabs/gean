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
func ProcessAttestations(state *types.State, attestations []*types.Attestation) *types.State {
	numValidators := uint64(len(state.Validators))

	// Deserialize justifications from SSZ form into working map.
	justifications := make(map[[32]byte][]bool)
	for i, root := range state.JustificationsRoots {
		votes := make([]bool, numValidators)
		for v := uint64(0); v < numValidators; v++ {
			bitIdx := uint64(i)*numValidators + v
			votes[v] = getBit(state.JustificationsValidators, bitIdx)
		}
		justifications[root] = votes
	}

	justifiedSlots := cloneBitlist(state.JustifiedSlots)
	latestJustified := &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot}
	latestFinalized := &types.Checkpoint{Root: state.LatestFinalized.Root, Slot: state.LatestFinalized.Slot}
	originalFinalizedSlot := state.LatestFinalized.Slot

	for _, att := range attestations {
		source := att.Data.Source
		target := att.Data.Target
		srcSlot := source.Slot
		tgtSlot := target.Slot

		// Target must be after source (strict).
		if tgtSlot <= srcSlot {
			continue
		}

		// Source must be justified.
		if srcSlot >= uint64(bitlistLen(justifiedSlots)) || !getBit(justifiedSlots, srcSlot) {
			continue
		}

		// Target must not already be justified.
		if tgtSlot < uint64(bitlistLen(justifiedSlots)) && getBit(justifiedSlots, tgtSlot) {
			continue
		}

		// Source root must match historical block hashes.
		if srcSlot >= uint64(len(state.HistoricalBlockHashes)) || state.HistoricalBlockHashes[srcSlot] != source.Root {
			continue
		}

		// Target root must match historical block hashes.
		if tgtSlot >= uint64(len(state.HistoricalBlockHashes)) || state.HistoricalBlockHashes[tgtSlot] != target.Root {
			continue
		}

		// Target must be justifiable after the original finalized slot.
		if !types.IsJustifiableAfter(tgtSlot, originalFinalizedSlot) {
			continue
		}

		// Validate validator ID.
		validatorID := att.ValidatorID
		if validatorID >= numValidators {
			continue
		}

		// Record vote (idempotent — skip if already voted).
		if _, ok := justifications[target.Root]; !ok {
			justifications[target.Root] = make([]bool, numValidators)
		}
		if justifications[target.Root][validatorID] {
			continue
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
			continue
		}

		// Justify target.
		latestJustified = &types.Checkpoint{Root: target.Root, Slot: tgtSlot}
		for uint64(bitlistLen(justifiedSlots)) <= tgtSlot {
			justifiedSlots = appendBit(justifiedSlots, false)
		}
		justifiedSlots = setBit(justifiedSlots, tgtSlot, true)
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
			latestFinalized = &types.Checkpoint{Root: source.Root, Slot: srcSlot}
		}
	}

	// Serialize justifications back to SSZ form.
	sortedRoots := sortedJustificationRoots(justifications)
	flatVotes := flattenVotes(sortedRoots, justifications, numValidators)

	out := copyState(state)
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
