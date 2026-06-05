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

	return VerifyStateRoot(state, block)
}

// VerifyStateRoot checks that the post-transition state root matches the
// block's committed state root.
func VerifyStateRoot(state *types.State, block *types.Block) error {
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
