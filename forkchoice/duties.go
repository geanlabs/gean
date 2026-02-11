package forkchoice

import (
	"fmt"

	"github.com/devylongs/gean/types"
	"github.com/devylongs/gean/validator"
)

// ProduceBlock creates a block using iterative (fixed-point) attestation collection.
// Iterates: build block -> apply state transition -> collect new attestations using
// post-state's LatestJustified as source -> repeat until no new attestations.
// Processing attestations may justify new checkpoints, making additional attestations
// valid. Typically converges in 1-2 iterations.
func (s *Store) ProduceBlock(slot types.Slot, validatorIndex types.ValidatorIndex) (*types.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	headState := s.States[s.Head]
	numValidators := uint64(len(headState.Validators))
	if err := validator.ValidateProposer(slot, validatorIndex, numValidators); err != nil {
		return nil, err
	}

	s.advanceToSlotLocked(slot)

	headRoot := s.Head
	headState, exists := s.States[headRoot]
	if !exists {
		return nil, fmt.Errorf("head state not found")
	}

	blockExists := func(root types.Root) bool { _, ok := s.Blocks[root]; return ok }

	// Iteratively collect attestations using fixed-point algorithm.
	var attestations []types.Attestation
	for {
		block, postState, err := validator.BuildBlock(slot, validatorIndex, headRoot, headState, attestations)
		if err != nil {
			return nil, err
		}

		// Find new attestations using the post-state's latest justified as source.
		newAttestations := validator.CollectNewAttestations(
			s.LatestKnownVotes,
			blockExists,
			postState.LatestJustified,
			attestations,
		)

		// Fixed point reached: no new attestations found.
		if len(newAttestations) == 0 {
			blockHash, err := block.HashTreeRoot()
			if err != nil {
				return nil, fmt.Errorf("hash block: %w", err)
			}
			s.Blocks[blockHash] = block
			s.States[blockHash] = postState
			s.updateHeadLocked()
			return block, nil
		}

		attestations = append(attestations, newAttestations...)
	}
}

// ProduceAttestationData creates attestation data for the given slot.
func (s *Store) ProduceAttestationData(slot types.Slot) *types.AttestationData {
	s.mu.Lock()

	s.advanceToSlotLocked(slot)
	headRoot := s.Head
	headBlock := s.Blocks[headRoot]

	headCheckpoint := types.Checkpoint{
		Root: headRoot,
		Slot: headBlock.Slot,
	}

	targetCheckpoint := s.getVoteTargetLocked()
	latestJustified := s.LatestJustified

	s.mu.Unlock()

	return &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: targetCheckpoint,
		Source: latestJustified,
	}
}
