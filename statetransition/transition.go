package statetransition

import (
	"time"

	"github.com/geanlabs/gean/types"
)

// StateTransition applies a block to a state, producing the post-block state.
// Steps: process_slots → process_block → verify state_root.
func StateTransition(state *types.State, block *types.Block) error {
	start := time.Now()

	// 1. Advance through empty slots to the block's slot.
	if err := ProcessSlots(state, block.Slot); err != nil {
		return err
	}

	// 2. Validate header and process attestations.
	if err := ProcessBlock(state, block); err != nil {
		return err
	}

	// 3. Verify computed state root matches the block's claim.
	computedRoot, err := state.HashTreeRoot()
	if err != nil {
		return err
	}
	if computedRoot != block.StateRoot {
		return &StateRootMismatchError{
			Expected: block.StateRoot,
			Computed: computedRoot,
		}
	}

	if ObserveTotalTimeHook != nil {
		ObserveTotalTimeHook(time.Since(start).Seconds())
	}
	return nil
}
