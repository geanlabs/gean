package transition

import (
	"fmt"

	"github.com/geanlabs/gean/internal/lean/types"
)

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

// ApplyBlock returns the state block leads to from state, leaving state as it
// is. It skips the state-root check, so it serves trial blocks whose root is not
// known yet.
func ApplyBlock(state *types.State, block *types.Block) (*types.State, error) {
	post, err := state.Clone()
	if err != nil {
		return nil, err
	}
	if err := ProcessSlots(post, block.Slot); err != nil {
		return nil, fmt.Errorf("process slots: %w", err)
	}
	if err := ProcessBlock(post, block); err != nil {
		return nil, fmt.Errorf("process block: %w", err)
	}
	return post, nil
}

// ApplyEmptyBlock returns the state an empty block at the next slot leads to,
// leaving state as it is.
func ApplyEmptyBlock(state *types.State) (*types.State, error) {
	// The block's parent is the latest header with the state's root filled in,
	// as processing the slot fills it.
	parent := *state.LatestBlockHeader
	if parent.StateRoot == types.ZeroRoot {
		root, err := state.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		parent.StateRoot = root
	}
	parentRoot, err := parent.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	next := state.Slot + 1
	return ApplyBlock(state, &types.Block{
		Slot:          next,
		ProposerIndex: types.ProposerIndex(next, state.NumValidators()),
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	})
}
