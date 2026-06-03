package store

import "github.com/geanlabs/gean/internal/types"

func copyAttestationData(data *types.AttestationData) *types.AttestationData {
	if data == nil {
		return nil
	}
	return &types.AttestationData{
		Slot:   data.Slot,
		Head:   copyCheckpoint(data.Head),
		Target: copyCheckpoint(data.Target),
		Source: copyCheckpoint(data.Source),
	}
}

func copyCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return nil
	}
	out := *cp
	return &out
}

func copyProof(proof *types.AggregatedSignatureProof) *types.AggregatedSignatureProof {
	if proof == nil {
		return nil
	}
	return &types.AggregatedSignatureProof{
		Participants: copyBytes(proof.Participants),
		ProofData:    copyBytes(proof.ProofData),
	}
}

func copyBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out
}
