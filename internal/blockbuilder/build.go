package blockbuilder

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func Build(input Input) (*Result, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	required := requiredCheckpoint(input.RequiredJustified)

	plan, err := planAttestations(input)
	if err != nil {
		return nil, err
	}

	finalBlock := newBlock(input.Slot, input.ProposerIndex, input.ParentRoot, plan.attestations)
	stateRoot, err := plan.postState.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("post-state root: %w", err)
	}
	finalBlock.StateRoot = stateRoot

	if !justifiedMeetsRequired(plan.postState, required) {
		return nil, errJustifiedDivergenceNotClosed(
			plan.postState.LatestJustified, required)
	}

	return &Result{
		Block:             finalBlock,
		AttestationProofs: plan.proofs,
		PayloadErrors:     plan.payloadErrors,
	}, nil
}

func validateInput(input Input) error {
	if err := validateHeadState(input.HeadState); err != nil {
		return err
	}
	if len(input.Payloads) > 0 && input.KnownBlockRoots == nil {
		return errMalformedInput("known block roots are nil")
	}
	return nil
}

func newBlock(slot, proposerIndex uint64, parentRoot [32]byte, attestations []*types.AggregatedAttestation) *types.Block {
	return &types.Block{
		Slot:          slot,
		ProposerIndex: proposerIndex,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{Attestations: attestations},
	}
}
