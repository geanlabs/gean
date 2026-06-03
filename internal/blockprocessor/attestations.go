package blockprocessor

import (
	"bytes"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func importBlockAttestations(s *store.ConsensusStore, signedBlock *types.SignedBlock) {
	if s == nil || s.KnownPayloads == nil || signedBlock == nil || signedBlock.Block == nil || signedBlock.Block.Body == nil || signedBlock.Signature == nil {
		return
	}

	for i, att := range signedBlock.Block.Body.Attestations {
		if !validAttestationShape(att) || i >= len(signedBlock.Signature.AttestationSignatures) {
			continue
		}
		proof := signedBlock.Signature.AttestationSignatures[i]
		if !proofMatchesAttestation(att, proof) {
			continue
		}
		dataRoot, err := att.Data.HashTreeRoot()
		if err != nil {
			continue
		}
		s.KnownPayloads.Push(dataRoot, att.Data, proof)
	}
}

func proofMatchesAttestation(att *types.AggregatedAttestation, proof *types.AggregatedSignatureProof) bool {
	return proof != nil &&
		types.BitlistCount(proof.Participants) > 0 &&
		bytes.Equal(att.AggregationBits, proof.Participants)
}
