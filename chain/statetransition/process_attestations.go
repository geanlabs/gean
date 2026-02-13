package statetransition

import (
	"github.com/geanlabs/gean/types"
)

// ProcessAttestations applies attestation votes and updates
// justification/finalization according to leanSpec devnet0 rules.
func ProcessAttestations(state *types.State, attestations []*types.SignedVote) *types.State {
	justifiedSlots := cloneBitlist(state.JustifiedSlots)
	latestJustified := &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot}
	latestFinalized := &types.Checkpoint{Root: state.LatestFinalized.Root, Slot: state.LatestFinalized.Slot}

	for _, sv := range attestations {
		vote := sv.Data
		source := vote.Source
		target := vote.Target

		if source.Slot >= target.Slot {
			continue
		}

		srcSlot := source.Slot
		tgtSlot := target.Slot

		// Source must be within justified history and marked justified.
		if srcSlot >= uint64(bitlistLen(justifiedSlots)) {
			continue
		}
		sourceIsJustified := getBit(justifiedSlots, srcSlot)
		if !sourceIsJustified {
			continue
		}

		targetAlreadyJustified := tgtSlot < uint64(bitlistLen(justifiedSlots)) && getBit(justifiedSlots, tgtSlot)
		if targetAlreadyJustified {
			if source.Slot+1 == target.Slot && latestJustified.Slot < target.Slot {
				latestFinalized = &types.Checkpoint{Root: source.Root, Slot: source.Slot}
				latestJustified = &types.Checkpoint{Root: target.Root, Slot: target.Slot}
			}
			continue
		}

		// Source is justified and target is not yet justified: justify target.
		for uint64(bitlistLen(justifiedSlots)) <= tgtSlot {
			justifiedSlots = appendBit(justifiedSlots, false)
		}
		justifiedSlots = setBit(justifiedSlots, tgtSlot, true)

		if target.Slot > latestJustified.Slot {
			latestJustified = &types.Checkpoint{Root: target.Root, Slot: target.Slot}
		}
	}

	out := copyState(state)
	out.JustifiedSlots = justifiedSlots
	out.LatestJustified = latestJustified
	out.LatestFinalized = latestFinalized
	return out
}
