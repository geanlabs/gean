package forkchoice

import (
	"bytes"

	"github.com/geanlabs/gean/types"
)

type signatureKey struct {
	validatorID uint64
	dataRoot    [32]byte
}

type storedSignature struct {
	slot      uint64
	signature [types.XMSSSignatureSize]byte
}

type storedAggregatedPayload struct {
	slot  uint64
	proof *types.AggregatedSignatureProof
}

func makeSignatureKey(validatorID uint64, data *types.AttestationData) (signatureKey, bool) {
	if data == nil {
		return signatureKey{}, false
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return signatureKey{}, false
	}
	return signatureKey{
		validatorID: validatorID,
		dataRoot:    dataRoot,
	}, true
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

func (c *Store) storeGossipSignatureLocked(sa *types.SignedAttestation) {
	if sa == nil || sa.Message == nil {
		return
	}
	key, ok := makeSignatureKey(sa.ValidatorID, sa.Message)
	if !ok {
		return
	}
	existing, exists := c.gossipSignatures[key]
	if !exists || existing.slot <= sa.Message.Slot {
		c.gossipSignatures[key] = storedSignature{
			slot:      sa.Message.Slot,
			signature: sa.Signature,
		}
	}
}

func (c *Store) storeAggregatedPayloadLocked(
	validatorID uint64,
	data *types.AttestationData,
	proof *types.AggregatedSignatureProof,
) {
	if proof == nil || data == nil {
		return
	}
	key, ok := makeSignatureKey(validatorID, data)
	if !ok {
		return
	}
	entry := storedAggregatedPayload{
		slot:  data.Slot,
		proof: cloneAggregatedSignatureProof(proof),
	}

	existing := c.aggregatedPayloads[key]
	for _, current := range existing {
		if current.proof == nil {
			continue
		}
		if bytes.Equal(current.proof.Participants, proof.Participants) &&
			bytes.Equal(current.proof.ProofData, proof.ProofData) {
			return
		}
	}

	existing = append(existing, entry)
	const maxProofsPerKey = 8
	if len(existing) > maxProofsPerKey {
		existing = existing[len(existing)-maxProofsPerKey:]
	}
	c.aggregatedPayloads[key] = existing
}
