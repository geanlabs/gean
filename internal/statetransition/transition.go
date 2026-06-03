package statetransition

import "github.com/geanlabs/gean/internal/types"

func StateTransition(state *types.State, block *types.Block) error {
	if block == nil {
		return malformedBlock("block")
	}
	if err := ProcessSlots(state, block.Slot); err != nil {
		return err
	}

	if err := ProcessBlock(state, block); err != nil {
		return err
	}

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

	return nil
}
