// Package validator implements block and vote production for the Lean protocol.
// Functions are pure — the caller (forkchoice.Store) manages state and locking.
package validator

import (
	"fmt"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/types"
)

// ValidateProposer checks round-robin proposer assignment: slot % num_validators.
func ValidateProposer(slot types.Slot, validatorIndex types.ValidatorIndex, numValidators uint64) error {
	expectedProposer := uint64(slot) % numValidators
	if uint64(validatorIndex) != expectedProposer {
		return fmt.Errorf("validator %d is not the proposer for slot %d (expected %d)",
			validatorIndex, slot, expectedProposer)
	}
	return nil
}

// CollectNewAttestations gathers votes from known validators for block inclusion,
// filtering out attestations already in the existing set.
func CollectNewAttestations(
	knownVotes []types.Checkpoint,
	blockExists func(types.Root) bool,
	latestJustified types.Checkpoint,
	existing []types.SignedVote,
) []types.SignedVote {
	// Build a set of existing attestation validator IDs for fast lookup.
	seen := make(map[uint64]bool, len(existing))
	for _, sv := range existing {
		seen[sv.Data.ValidatorID] = true
	}

	var newAttestations []types.SignedVote

	for validatorID, checkpoint := range knownVotes {
		if checkpoint.Root.IsZero() {
			continue
		}
		if !blockExists(checkpoint.Root) {
			continue
		}
		if seen[uint64(validatorID)] {
			continue
		}

		signedVote := types.SignedVote{
			Data: types.Vote{
				ValidatorID: uint64(validatorID),
				Slot:        checkpoint.Slot,
				Head:        checkpoint,
				Target:      checkpoint,
				Source:       latestJustified,
			},
			Signature: types.Root{},
		}
		newAttestations = append(newAttestations, signedVote)
	}

	return newAttestations
}

// BuildBlock creates a block, applies state transition, and fills the state root.
func BuildBlock(
	slot types.Slot,
	validatorIndex types.ValidatorIndex,
	parentRoot types.Root,
	headState *types.State,
	attestations []types.SignedVote,
) (*types.Block, *types.State, error) {
	finalState, err := consensus.ProcessSlots(headState, slot)
	if err != nil {
		return nil, nil, fmt.Errorf("process slots: %w", err)
	}

	block := &types.Block{
		Slot:          slot,
		ProposerIndex: uint64(validatorIndex),
		ParentRoot:    parentRoot,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: attestations},
	}

	postState, err := consensus.ProcessBlock(finalState, block)
	if err != nil {
		return nil, nil, fmt.Errorf("process block: %w", err)
	}

	stateRoot, err := postState.HashTreeRoot()
	if err != nil {
		return nil, nil, fmt.Errorf("hash state: %w", err)
	}
	block.StateRoot = stateRoot

	return block, postState, nil
}

// BuildVote creates an attestation vote for the given slot and validator.
func BuildVote(
	slot types.Slot,
	validatorIndex types.ValidatorIndex,
	headCheckpoint types.Checkpoint,
	targetCheckpoint types.Checkpoint,
	sourceCheckpoint types.Checkpoint,
) *types.Vote {
	return &types.Vote{
		ValidatorID: uint64(validatorIndex),
		Slot:        slot,
		Head:        headCheckpoint,
		Target:      targetCheckpoint,
		Source:       sourceCheckpoint,
	}
}
