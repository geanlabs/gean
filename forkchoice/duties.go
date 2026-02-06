package forkchoice

import (
	"fmt"

	"github.com/devylongs/gean/types"
	"github.com/devylongs/gean/validator"
)

// ProduceBlock creates a new block for the given slot and validator.
func (s *Store) ProduceBlock(slot types.Slot, validatorIndex types.ValidatorIndex) (*types.Block, error) {
	if err := validator.ValidateProposer(slot, validatorIndex, s.Config.NumValidators); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.advanceToSlotLocked(slot)

	headState, exists := s.States[s.Head]
	if !exists {
		return nil, fmt.Errorf("head state not found")
	}

	attestations := validator.CollectAttestations(
		s.LatestKnownVotes,
		func(root types.Root) bool { _, ok := s.Blocks[root]; return ok },
		headState.LatestJustified,
	)

	block, postState, err := validator.BuildBlock(slot, validatorIndex, s.Head, headState, attestations)
	if err != nil {
		return nil, err
	}

	blockHash, err := block.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash block: %w", err)
	}

	s.Blocks[blockHash] = block
	s.States[blockHash] = postState
	s.updateHeadLocked()

	return block, nil
}

// ProduceAttestationVote creates an attestation vote for the given slot and validator.
func (s *Store) ProduceAttestationVote(slot types.Slot, validatorIndex types.ValidatorIndex) *types.Vote {
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

	return validator.BuildVote(slot, validatorIndex, headCheckpoint, targetCheckpoint, latestJustified)
}
