package forkchoice

import (
	"bytes"

	"github.com/geanlabs/gean/types"
)

type aggregatedPayload struct {
	data   *types.AttestationData
	proofs []*types.AggregatedSignatureProof
}

func makeAttestationDataKey(data *types.AttestationData) ([32]byte, bool) {
	if data == nil {
		return [32]byte{}, false
	}
	root, err := data.HashTreeRoot()
	if err != nil {
		return [32]byte{}, false
	}
	return root, true
}

func sameAggregatedProof(a, b *types.AggregatedSignatureProof) bool {
	if a == nil || b == nil {
		return false
	}
	return bytes.Equal(a.Participants, b.Participants) && bytes.Equal(a.ProofData, b.ProofData)
}

func addAggregatedPayload(dst map[[32]byte]aggregatedPayload, data *types.AttestationData, proof *types.AggregatedSignatureProof) {
	if data == nil || proof == nil {
		return
	}
	key, ok := makeAttestationDataKey(data)
	if !ok {
		return
	}

	payload := dst[key]
	if payload.data == nil {
		payload.data = data
	}
	for _, existing := range payload.proofs {
		if sameAggregatedProof(existing, proof) {
			dst[key] = payload
			return
		}
	}
	payload.proofs = append(payload.proofs, cloneAggregatedSignatureProof(proof))
	dst[key] = payload
}

func mergeAggregatedPayloads(dst map[[32]byte]aggregatedPayload, src map[[32]byte]aggregatedPayload) map[[32]byte]aggregatedPayload {
	if dst == nil {
		dst = make(map[[32]byte]aggregatedPayload)
	}
	for _, payload := range src {
		if payload.data == nil {
			continue
		}
		for _, proof := range payload.proofs {
			addAggregatedPayload(dst, payload.data, proof)
		}
	}
	return dst
}

func extractAttestationsFromAggregatedPayloads(payloads map[[32]byte]aggregatedPayload) map[uint64]*types.SignedAttestation {
	attestations := make(map[uint64]*types.SignedAttestation)
	for _, payload := range payloads {
		if payload.data == nil {
			continue
		}
		for _, proof := range payload.proofs {
			if proof == nil {
				continue
			}
			for _, vid := range bitlistToValidatorIDs(proof.Participants) {
				sa := &types.SignedAttestation{ValidatorID: vid, Message: payload.data}
				existing := attestations[vid]
				if existing == nil || existing.Message == nil || existing.Message.Slot < payload.data.Slot {
					attestations[vid] = sa
				}
			}
		}
	}
	return attestations
}
