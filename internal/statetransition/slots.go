package statetransition

import "github.com/geanlabs/gean/internal/types"

func ProcessSlots(state *types.State, targetSlot uint64) error {
	if state == nil {
		return malformedState("state")
	}
	if state.LatestBlockHeader == nil {
		return malformedState("latest block header")
	}
	if state.Slot >= targetSlot {
		return &StateSlotIsNewerError{TargetSlot: targetSlot, CurrentSlot: state.Slot}
	}

	if state.LatestBlockHeader.StateRoot == types.ZeroRoot {
		root, err := state.HashTreeRoot()
		if err != nil {
			return err
		}
		state.LatestBlockHeader.StateRoot = root
	}

	state.Slot = targetSlot
	return nil
}
