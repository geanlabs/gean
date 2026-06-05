package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func validateStore(s *store.ConsensusStore) error {
	if s == nil {
		return fmt.Errorf("consensus store is nil")
	}
	if s.Backend == nil {
		return fmt.Errorf("consensus store backend is nil")
	}
	return nil
}

func validateSignedBlock(signedBlock *types.SignedBlock, verify bool) (*types.Block, error) {
	if signedBlock == nil {
		return nil, fmt.Errorf("malformed signed block: signed block is nil")
	}
	if signedBlock.Block == nil {
		return nil, fmt.Errorf("malformed signed block: block is nil")
	}
	if signedBlock.Block.Body == nil {
		return nil, fmt.Errorf("malformed block: body is nil")
	}
	if verify && signedBlock.Signature == nil {
		return nil, &store.StoreError{
			Kind:    store.ErrAttestationSignatureMismatch,
			Message: "block signatures missing",
		}
	}
	return signedBlock.Block, nil
}

func validateBlockAttestations(block *types.Block) error {
	seen := make(map[[32]byte]bool)
	for _, att := range block.Body.Attestations {
		if !validAttestationShape(att) {
			return fmt.Errorf("malformed block attestation")
		}

		dataRoot, err := att.Data.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("hash attestation data: %w", err)
		}
		if seen[dataRoot] {
			return &store.StoreError{
				Kind:    store.ErrDuplicateAttestationData,
				Message: "block contains duplicate AttestationData",
			}
		}
		seen[dataRoot] = true
	}

	if len(seen) > int(types.MaxAttestationsData) {
		return &store.StoreError{
			Kind:    store.ErrTooManyAttestationData,
			Message: fmt.Sprintf("block has %d distinct AttestationData (max %d)", len(seen), types.MaxAttestationsData),
		}
	}
	return nil
}

func validAttestationShape(att *types.AggregatedAttestation) bool {
	if att == nil || att.Data == nil {
		return false
	}
	return att.Data.Head != nil && att.Data.Target != nil && att.Data.Source != nil
}
