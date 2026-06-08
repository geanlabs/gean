package statetransition

import (
	"github.com/geanlabs/gean/internal/types"
)

func ProcessAttestations(state *types.State, attestations []*types.AggregatedAttestation) error {
	if state == nil {
		return malformedState("state")
	}
	if state.LatestJustified == nil {
		return malformedState("latest justified")
	}
	if state.LatestFinalized == nil {
		return malformedState("latest finalized")
	}

	validatorCount := int(state.NumValidators())
	if validatorCount == 0 {
		return nil
	}

	for _, root := range state.JustificationsRoots {
		var r [32]byte
		copy(r[:], root)
		if types.IsZeroRoot(r) {
			return ErrZeroHashInJustificationRoots
		}
	}

	justifications := reconstructJustifications(state, validatorCount)
	rootToSlot := buildRootToSlot(state)

	for _, agg := range attestations {
		if !validAttestationShape(agg) {
			continue
		}
		source := agg.Data.Source
		target := agg.Data.Target

		if !IsValidVote(state, source, target) {
			continue
		}
		if !HeadMatchesChain(state, agg.Data.Head) {
			continue
		}

		votes, exists := justifications[target.Root]
		if !exists {
			votes = make([]bool, validatorCount)
			justifications[target.Root] = votes
		}

		bitsLen := types.BitlistLen(agg.AggregationBits)
		if bitsLen > uint64(validatorCount) {
			continue
		}

		for i := range bitsLen {
			if types.BitlistGet(agg.AggregationBits, i) {
				votes[i] = true
			}
		}

		voteCount := countTrue(votes)
		if 3*voteCount >= 2*validatorCount {
			if target.Slot > state.LatestJustified.Slot {
				state.LatestJustified = copyCheckpoint(target)
			}
			setSlotJustified(state, state.LatestFinalized.Slot, target.Slot)

			delete(justifications, target.Root)
			tryFinalize(state, source, target, &justifications, rootToSlot)
		}
	}

	serializeJustifications(state, justifications, validatorCount)

	return nil
}
