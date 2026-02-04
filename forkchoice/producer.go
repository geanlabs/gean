package forkchoice

import (
	"fmt"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/types"
)

// ProduceBlock creates a new block for the given slot and validator.
// It iteratively collects valid attestations and computes the state root.
func (s *Store) ProduceBlock(slot types.Slot, validatorIndex types.ValidatorIndex) (*types.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateProposer(slot, validatorIndex); err != nil {
		return nil, err
	}

	s.advanceToSlotLocked(slot)

	headState, err := s.getHeadState()
	if err != nil {
		return nil, err
	}

	attestations := s.collectAttestations(headState)

	return s.buildBlock(slot, validatorIndex, headState, attestations)
}

func (s *Store) validateProposer(slot types.Slot, validatorIndex types.ValidatorIndex) error {
	expectedProposer := uint64(slot) % s.Config.NumValidators
	if uint64(validatorIndex) != expectedProposer {
		return fmt.Errorf("validator %d is not the proposer for slot %d (expected %d)",
			validatorIndex, slot, expectedProposer)
	}
	return nil
}

func (s *Store) getHeadState() (*types.State, error) {
	headState, exists := s.States[s.Head]
	if !exists {
		return nil, fmt.Errorf("head state not found")
	}
	return headState, nil
}

// collectAttestations gathers votes from known validators for block inclusion.
// For devnet0: no aggregation, just collect all known votes.
func (s *Store) collectAttestations(headState *types.State) []types.SignedVote {
	var attestations []types.SignedVote

	for validatorID, checkpoint := range s.LatestKnownVotes {
		if checkpoint.Root.IsZero() {
			continue
		}
		if _, exists := s.Blocks[checkpoint.Root]; !exists {
			continue
		}

		signedVote := types.SignedVote{
			ValidatorID: uint64(validatorID),
			Message: types.Vote{
				Slot:   checkpoint.Slot,
				Head:   checkpoint,
				Target: checkpoint,
				Source: headState.LatestJustified,
			},
			Signature: [4000]byte{},
		}
		attestations = append(attestations, signedVote)
	}

	return attestations
}

func (s *Store) buildBlock(slot types.Slot, validatorIndex types.ValidatorIndex, headState *types.State, attestations []types.SignedVote) (*types.Block, error) {
	finalState, err := consensus.ProcessSlots(headState, slot)
	if err != nil {
		return nil, fmt.Errorf("process slots for final block: %w", err)
	}

	block := &types.Block{
		Slot:          slot,
		ProposerIndex: uint64(validatorIndex),
		ParentRoot:    s.Head,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: attestations},
	}

	postState, err := consensus.ProcessBlock(finalState, block)
	if err != nil {
		return nil, fmt.Errorf("process final block: %w", err)
	}

	stateRoot, err := postState.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash final state: %w", err)
	}
	block.StateRoot = stateRoot

	// Store block and state
	blockHash, err := block.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash final block: %w", err)
	}
	s.Blocks[blockHash] = block
	s.States[blockHash] = postState

	// Update head to include our new block
	s.updateHeadLocked()

	return block, nil
}

// ProduceAttestationVote creates an attestation vote for the given slot.
// The caller is responsible for wrapping this in SignedVote with ValidatorID.
func (s *Store) ProduceAttestationVote(slot types.Slot) *types.Vote {
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

	return &types.Vote{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: targetCheckpoint,
		Source: latestJustified,
	}
}
