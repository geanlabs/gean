// Package zkstf defines what a zkVM proves about gean's state transition: that
// applying an unsigned block to a pre-state succeeds and yields the block's
// state root. Signatures and the leanVM block proof are checked on import,
// outside the transition. The zkVM guests run a Rust port of the transition
// (zk/stf); Apply is the Go reference every guest result is compared with.
package zkstf

import (
	"fmt"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

// Apply decodes a framed input, runs the state transition and returns the
// public values. Every guest computes the same; any error must make the guest
// halt as a failure so that no valid proof exists for the input.
func Apply(input []byte) (PublicValues, error) {
	var pv PublicValues

	stateSSZ, blockSSZ, err := DecodeInput(input)
	if err != nil {
		return pv, err
	}
	state := new(types.State)
	if err := state.UnmarshalSSZ(stateSSZ); err != nil {
		return pv, fmt.Errorf("%w: state: %v", ErrMalformedInput, err)
	}
	block := new(types.Block)
	if err := block.UnmarshalSSZ(blockSSZ); err != nil {
		return pv, fmt.Errorf("%w: block: %v", ErrMalformedInput, err)
	}

	if pv.PreStateRoot, err = state.HashTreeRoot(); err != nil {
		return pv, fmt.Errorf("pre-state root: %w", err)
	}
	if pv.BlockRoot, err = block.HashTreeRoot(); err != nil {
		return pv, fmt.Errorf("block root: %w", err)
	}
	if err := statetransition.StateTransition(state, block); err != nil {
		return pv, err
	}
	// StateTransition has verified HTR(post-state) == block.StateRoot, so the
	// block's root is the post-state root without hashing the state again.
	pv.PostStateRoot = block.StateRoot
	return pv, nil
}
