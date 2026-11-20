package importer

import (
	"time"

	"github.com/geanlabs/gean/internal/core/metrics"
	"github.com/geanlabs/gean/internal/lean/transition"
	"github.com/geanlabs/gean/internal/lean/types"
)

func transitionState(parentState *types.State, block *types.Block) (*types.State, error) {
	state, err := parentState.Clone()
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, transition.StateTransition(state, block)
	}

	// Phase-by-phase to record per-phase state-transition metrics. The
	// composition mirrors transition.StateTransition; the spec logic
	// itself stays in the (pure) transition package.
	slotsBefore := state.Slot
	slotsStart := time.Now()
	if err := transition.ProcessSlots(state, block.Slot); err != nil {
		return nil, err
	}
	metrics.ObserveSTFSlotsTime(time.Since(slotsStart).Seconds())
	if block.Slot > slotsBefore {
		metrics.IncSTFSlotsProcessed(block.Slot - slotsBefore)
	}

	blockStart := time.Now()
	if err := transition.ProcessBlockHeader(state, block); err != nil {
		return nil, err
	}
	attestations := block.Body.Attestations
	attStart := time.Now()
	if err := transition.ProcessAttestations(state, attestations); err != nil {
		return nil, err
	}
	metrics.ObserveSTFAttestationsTime(time.Since(attStart).Seconds())
	metrics.IncSTFAttestationsProcessed(len(attestations))
	metrics.ObserveSTFBlockTime(time.Since(blockStart).Seconds())

	if err := transition.VerifyStateRoot(state, block); err != nil {
		return nil, err
	}
	return state, nil
}
