package forkchoice

import (
	"bytes"

	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

type signatureKey struct {
	validatorID uint64
	dataRoot    [32]byte
}

type storedSignature struct {
	slot      uint64
	data      *types.AttestationData
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

func (c *Store) updateCacheMetricsLocked() {
	metrics.GossipSignaturesCount.Set(float64(len(c.gossipSignatures)))
	metrics.LatestKnownAggregatedPayloads.Set(float64(len(c.latestKnownAggregatedPayloads)))
	metrics.LatestNewAggregatedPayloads.Set(float64(len(c.latestNewAggregatedPayloads)))
	metrics.AggregatedPayloadCacheKeys.Set(float64(len(c.aggregatedPayloads)))
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
			data:      sa.Message,
			signature: sa.Signature,
		}
	}
	c.updateCacheMetricsLocked()
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
	c.updateCacheMetricsLocked()
}

func (c *Store) pruneEphemeralCachesLocked(finalizedSlot uint64) int {
	pruned := 0
	for key, stored := range c.gossipSignatures {
		if stored.slot <= finalizedSlot {
			delete(c.gossipSignatures, key)
			pruned++
		}
	}

	for key, entries := range c.aggregatedPayloads {
		kept := entries[:0]
		for _, entry := range entries {
			if entry.slot > finalizedSlot {
				kept = append(kept, entry)
				continue
			}
			pruned++
		}
		if len(kept) == 0 {
			delete(c.aggregatedPayloads, key)
			continue
		}
		c.aggregatedPayloads[key] = kept
	}

	for key, payload := range c.latestKnownAggregatedPayloads {
		if payload.data == nil || payload.data.Slot <= finalizedSlot {
			delete(c.latestKnownAggregatedPayloads, key)
			pruned++
		}
	}

	for key, payload := range c.latestNewAggregatedPayloads {
		if payload.data == nil || payload.data.Slot <= finalizedSlot {
			delete(c.latestNewAggregatedPayloads, key)
			pruned++
		}
	}

	c.updateCacheMetricsLocked()
	return pruned
}

func (c *Store) PruneEphemeralCaches(finalizedSlot uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pruneEphemeralCachesLocked(finalizedSlot)
}
