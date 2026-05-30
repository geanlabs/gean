package node

import "github.com/geanlabs/gean/types"

func cloneCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return nil
	}
	clone := *cp
	return &clone
}

func cloneAttestationData(data *types.AttestationData) *types.AttestationData {
	if data == nil {
		return nil
	}
	return &types.AttestationData{
		Slot:   data.Slot,
		Head:   cloneCheckpoint(data.Head),
		Target: cloneCheckpoint(data.Target),
		Source: cloneCheckpoint(data.Source),
	}
}

func cloneAggregatedSignatureProof(proof *types.AggregatedSignatureProof) *types.AggregatedSignatureProof {
	if proof == nil {
		return nil
	}
	return &types.AggregatedSignatureProof{
		Participants: append([]byte(nil), proof.Participants...),
		ProofData:    append([]byte(nil), proof.ProofData...),
	}
}

func clonePayloadEntry(entry *PayloadEntry) *PayloadEntry {
	if entry == nil {
		return nil
	}
	clone := &PayloadEntry{
		Data:   cloneAttestationData(entry.Data),
		Proofs: make([]*types.AggregatedSignatureProof, 0, len(entry.Proofs)),
	}
	for _, proof := range entry.Proofs {
		clone.Proofs = append(clone.Proofs, cloneAggregatedSignatureProof(proof))
	}
	return clone
}

func cloneAttestationDataEntry(entry *AttestationDataEntry) *AttestationDataEntry {
	if entry == nil {
		return nil
	}
	clone := &AttestationDataEntry{
		Data:       cloneAttestationData(entry.Data),
		Signatures: make([]AttestationSignatureEntry, len(entry.Signatures)),
	}
	for i, sig := range entry.Signatures {
		clone.Signatures[i] = AttestationSignatureEntry{
			ValidatorID: sig.ValidatorID,
			Signature:   sig.Signature,
		}
	}
	return clone
}
