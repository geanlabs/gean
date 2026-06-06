package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func verifyBlockSignatures(
	s *store.ConsensusStore,
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

	valid, err := xmss.VerifySignatureSSZ(proposer.ProposalPubkey, slot, blockRoot, signedBlock.Signature.ProposerSignature)
	if err != nil {
		return &store.StoreError{Kind: store.ErrProposerSignatureDecodingFailed, Message: fmt.Sprintf("proposer sig decode: %v", err)}
	}
	if !valid {
		return &store.StoreError{Kind: store.ErrProposerSignatureVerificationFailed, Message: "proposer signature invalid"}
	}

	jobs, err := buildVerifyJobs(s, block, signedBlock.Signature)
	if err != nil {
		return err
	}
	return runVerifyJobs(jobs)
}
