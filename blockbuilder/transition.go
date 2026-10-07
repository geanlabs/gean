package blockbuilder

import (
	"fmt"

	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/types"
)

func validateHeadState(state *types.State) error {
	if state == nil {
		return errMalformedHeadState("state")
	}
	if state.LatestBlockHeader == nil {
		return errMalformedHeadState("latest block header")
	}
	if state.LatestJustified == nil {
		return errMalformedHeadState("latest justified")
	}
	if state.LatestFinalized == nil {
		return errMalformedHeadState("latest finalized")
	}
	return nil
}

func transitionBlock(headState *types.State, slot uint64, block *types.Block) (*types.State, error) {
	state, err := headState.Clone()
	if err != nil {
		return nil, err
	}
	if err := statetransition.ProcessSlots(state, slot); err != nil {
		return nil, fmt.Errorf("process slots: %w", err)
	}
	if err := statetransition.ProcessBlock(state, block); err != nil {
		return nil, fmt.Errorf("process block: %w", err)
	}
	return state, nil
}
