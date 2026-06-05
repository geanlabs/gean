package blockprocessor

import (
	"bytes"
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func validateProofForAttestation(
	attIdx int,
	att *types.AggregatedAttestation,
	proof *types.AggregatedSignatureProof,
) error {
	if proof == nil {
		return &store.StoreError{
			Kind:    store.ErrAttestationSignatureMismatch,
			Message: fmt.Sprintf("attestation %d has nil signature proof", attIdx),
		}
	}
	if len(proof.ProofData) == 0 {
		return &store.StoreError{
			Kind:    store.ErrAttestationSignatureMismatch,
			Message: fmt.Sprintf("attestation %d has empty signature proof", attIdx),
		}
	}
	if types.BitlistCount(proof.Participants) == 0 {
		return &store.StoreError{
			Kind:    store.ErrParticipantsMismatch,
			Message: fmt.Sprintf("attestation %d proof has no participants", attIdx),
		}
	}
	if !bytes.Equal(att.AggregationBits, proof.Participants) {
		return &store.StoreError{
			Kind:    store.ErrParticipantsMismatch,
			Message: fmt.Sprintf("attestation %d participants do not match proof", attIdx),
		}
	}
	return nil
}
