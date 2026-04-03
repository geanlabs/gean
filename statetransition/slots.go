package statetransition

import (
	"github.com/geanlabs/gean/types"
)

// ProcessSlots advances the state to targetSlot, caching the state root in the
// block header if it hasn't been set yet.
func ProcessSlots(state *types.State, targetSlot uint64) error {
	if state.Slot >= targetSlot {
		return &StateSlotIsNewerError{TargetSlot: targetSlot, CurrentSlot: state.Slot}
	}

	// Cache state root in latest_block_header if zero (first call after genesis).
	if state.LatestBlockHeader.StateRoot == types.ZeroRoot {
		root, err := state.HashTreeRoot()
		if err != nil {
			return err
		}
		state.LatestBlockHeader.StateRoot = root
	}

	// Advance directly to target slot.
	state.Slot = targetSlot
	return nil
}
