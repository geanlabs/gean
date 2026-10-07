package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func verifyBlockSignatures(
	scheme crypto.Scheme,
	signedBlock *types.SignedBlock,
	state *types.State,
) error {
	block, err := validateSignedBlock(signedBlock, true)
	if err != nil {
		return err
	}
	if state == nil {
		return &store.StoreError{Kind: store.ErrMissingParentState, Message: "parent state missing"}
	}
	if scheme == nil {
		return &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: "signature scheme missing"}
	}

	pubkeys := make([][]crypto.PublicKey, 0, len(block.Body.Attestations)+1)
	bindings := make([]crypto.Binding, 0, len(block.Body.Attestations)+1)
	for i, att := range block.Body.Attestations {
		indices := types.BitlistIndices(att.AggregationBits)
		if len(indices) == 0 {
			return &store.StoreError{Kind: store.ErrParticipantsMismatch, Message: fmt.Sprintf("attestation %d has no participants", i)}
		}
		keys := make([]crypto.PublicKey, 0, len(indices))
		for _, index := range indices {
			validator, err := validatorAt(state, index)
			if err != nil {
				return err
			}
			keys = append(keys, validator.AttestationPubkey)
		}
		root, err := att.Data.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("hash attestation %d data: %w", i, err)
		}
		slot, err := slot32(att.Data.Slot)
		if err != nil {
			return &store.StoreError{Kind: store.ErrSignatureDecodingFailed, Message: fmt.Sprintf("attestation %d slot: %v", i, err)}
		}
		pubkeys = append(pubkeys, keys)
		bindings = append(bindings, crypto.Binding{Message: root, Slot: slot})
	}

	proposer, err := validatorAt(state, block.ProposerIndex)
	if err != nil {
		return err
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}
	slot, err := slot32(block.Slot)
	if err != nil {
		return &store.StoreError{Kind: store.ErrProposerSignatureDecodingFailed, Message: fmt.Sprintf("proposer slot: %v", err)}
	}
	pubkeys = append(pubkeys, []crypto.PublicKey{proposer.ProposalPubkey})
	bindings = append(bindings, crypto.Binding{Message: blockRoot, Slot: slot})

	if err := scheme.VerifyBlockProof(signedBlock.Proof.Proof, pubkeys, bindings); err != nil {
		return &store.StoreError{Kind: store.ErrAggregateVerificationFailed, Message: fmt.Sprintf("block proof: %v", err)}
	}
	return nil
}
