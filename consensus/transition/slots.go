// slots.go contains slot-advancement transition logic.
//
// Package transition implements the consensus state transition function.
package transition

import (
	"fmt"

	"github.com/devylongs/gean/types"
)

// ProcessSlots advances the state through empty slots up to targetSlot.
func ProcessSlots(s *types.State, targetSlot types.Slot) (*types.State, error) {
	if s.Slot >= targetSlot {
		return nil, fmt.Errorf("target slot %d must be greater than current slot %d", targetSlot, s.Slot)
	}

	state := Copy(s)
	for state.Slot < targetSlot {
		// Cache state root into the latest header before advancing the slot.
		// This avoids circular dependency during block construction.
		if state.LatestBlockHeader.StateRoot.IsZero() {
			stateRoot, err := state.HashTreeRoot()
			if err != nil {
				return nil, fmt.Errorf("hash state: %w", err)
			}
			state.LatestBlockHeader.StateRoot = stateRoot
		}
		state.Slot++
	}
	return state, nil
}
