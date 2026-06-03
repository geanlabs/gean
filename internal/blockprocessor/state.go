package blockprocessor

import (
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func transitionState(parentState *types.State, block *types.Block) (*types.State, error) {
	state, err := parentState.Clone()
	if err != nil {
		return nil, err
	}
	if err := statetransition.StateTransition(state, block); err != nil {
		return nil, err
	}
	return state, nil
}
