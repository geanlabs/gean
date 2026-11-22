package blockbuilder

import (
	"github.com/geanlabs/gean/internal/lean/types"
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
