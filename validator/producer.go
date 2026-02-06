// Package validator implements block and vote production for the Lean protocol.
//
// Functions in this package are pure: they take inputs, produce outputs, and
// do not manage locks or mutable state. The caller (typically forkchoice.Store)
// is responsible for providing the correct inputs and storing results.
package validator

import (
	"fmt"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/types"
)

// ValidateProposer checks whether the given validator is the proposer for the slot.
func ValidateProposer(slot types.Slot, validatorIndex types.ValidatorIndex, numValidators uint64) error {
	expectedProposer := uint64(slot) % numValidators
	if uint64(validatorIndex) != expectedProposer {
		return fmt.Errorf("validator %d is not the proposer for slot %d (expected %d)",
			validatorIndex, slot, expectedProposer)
	}
	return nil
}

// CollectAttestations gathers votes from known validators for block inclusion.
// blockExists should return true if the block with the given root is known to the store.
func CollectAttestations(
	knownVotes []types.Checkpoint,
	blockExists func(types.Root) bool,
	latestJustified types.Checkpoint,
) []types.SignedVote {
	var attestations []types.SignedVote

	for validatorID, checkpoint := range knownVotes {
		if checkpoint.Root.IsZero() {
			continue
		}
		if !blockExists(checkpoint.Root) {
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
		attestations = append(attestations, signedVote)
	}

	return attestations
}

// BuildBlock creates a new block by processing slots up to the target and applying the block body.
// Returns the block (with state root filled) and the post-state.
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
