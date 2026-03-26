package forkchoice

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leanmultisig"
)

// AggregateCommitteeSignatures collects gossip signatures, builds aggregated
// proofs, and returns SignedAggregatedAttestation objects ready for publishing.
// Called by aggregators at interval 2.
func (c *Store) AggregateCommitteeSignatures() ([]*types.SignedAggregatedAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	headState, ok := c.storage.GetState(c.head)
	if !ok {
		return nil, fmt.Errorf("head state not found")
	}

	// Collect full attestations from gossip signatures.
	var attestations []*types.SignedAttestation
	for key, stored := range c.gossipSignatures {
		if stored.data == nil {
			continue
		}
		attestations = append(attestations, &types.SignedAttestation{
			ValidatorID: key.validatorID,
			Message:     stored.data,
			Signature:   stored.signature,
		})
	}

	if len(attestations) == 0 {
		return nil, nil
	}

	aggAtts, aggProofs, err := c.buildAggregatedAttestationsFromSigned(headState, attestations)
	if err != nil {
		return nil, fmt.Errorf("build aggregated attestations: %w", err)
	}

	result := make([]*types.SignedAggregatedAttestation, 0, len(aggAtts))
	for i, att := range aggAtts {
		if i >= len(aggProofs) || aggProofs[i] == nil {
			continue
		}
		result = append(result, &types.SignedAggregatedAttestation{
			Data:  att.Data,
			Proof: aggProofs[i],
		})
	}

	// Clear consumed gossip signatures (spec: remove aggregated entries).
	c.gossipSignatures = make(map[signatureKey]storedSignature)
	c.updateCacheMetricsLocked()

	return result, nil
}

func bitlistToValidatorIDs(bits []byte) []uint64 {
	numBits := uint64(statetransition.BitlistLen(bits))
	validatorIDs := make([]uint64, 0, numBits)
	for i := uint64(0); i < numBits; i++ {
		if statetransition.GetBit(bits, i) {
			validatorIDs = append(validatorIDs, i)
		}
	}
	return validatorIDs
}

func bitlistsEqual(a, b []byte) bool {
	aLen := statetransition.BitlistLen(a)
	bLen := statetransition.BitlistLen(b)
	if aLen != bLen {
		return false
	}
	for i := 0; i < aLen; i++ {
		idx := uint64(i)
		if statetransition.GetBit(a, idx) != statetransition.GetBit(b, idx) {
			return false
		}
	}
	return true
}

func (c *Store) buildAggregatedAttestationsFromSigned(
	state *types.State,
	attestations []*types.SignedAttestation,
) ([]*types.AggregatedAttestation, []*types.AggregatedSignatureProof, error) {
	if len(attestations) == 0 {
		return []*types.AggregatedAttestation{}, []*types.AggregatedSignatureProof{}, nil
	}

	// Group by attestation data root and keep at most one attestation per validator per root.
	grouped := make(map[[32]byte]map[uint64]*types.SignedAttestation)
	dataByRoot := make(map[[32]byte]*types.AttestationData)
	for _, sa := range attestations {
		if sa == nil || sa.Message == nil {
			continue
		}
		dataRoot, err := sa.Message.HashTreeRoot()
		if err != nil {
			return nil, nil, fmt.Errorf("hash attestation data: %w", err)
		}

		if _, ok := grouped[dataRoot]; !ok {
			grouped[dataRoot] = make(map[uint64]*types.SignedAttestation)
			dataByRoot[dataRoot] = sa.Message
		}
		existing, ok := grouped[dataRoot][sa.ValidatorID]
		if !ok || existing == nil || (existing.Message != nil && existing.Message.Slot < sa.Message.Slot) {
			grouped[dataRoot][sa.ValidatorID] = sa
		}
	}

	if len(grouped) == 0 {
		return []*types.AggregatedAttestation{}, []*types.AggregatedSignatureProof{}, nil
	}

	roots := make([][32]byte, 0, len(grouped))
	for root := range grouped {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return bytes.Compare(roots[i][:], roots[j][:]) < 0
	})

	aggregatedAttestations := make([]*types.AggregatedAttestation, 0, len(roots))
	attestationProofs := make([]*types.AggregatedSignatureProof, 0, len(roots))
	proverReady := false
	for _, root := range roots {
		group := grouped[root]
		validatorIDs := make([]uint64, 0, len(group))
		for validatorID := range group {
			validatorIDs = append(validatorIDs, validatorID)
		}
		sort.Slice(validatorIDs, func(i, j int) bool { return validatorIDs[i] < validatorIDs[j] })
		if len(validatorIDs) == 0 {
			continue
		}

		data := dataByRoot[root]
		bits := makeAggregationBits(validatorIDs)
		if cached := c.findReusableAggregatedProof(data, validatorIDs, bits); cached != nil {
			aggregatedAttestations = append(aggregatedAttestations, &types.AggregatedAttestation{
				AggregationBits: bits,
				Data:            data,
			})
			attestationProofs = append(attestationProofs, cached)
			log.Info(
				"attestation aggregate proof reused (leanMultisig)",
				"slot", data.Slot,
				"participants", len(validatorIDs),
				"proof_size", fmt.Sprintf("%d bytes", len(cached.ProofData)),
			)
			continue
		}

		signerIDs := make([]uint64, 0, len(validatorIDs))
		pubkeys := make([][]byte, 0, len(validatorIDs))
		signatures := make([][]byte, 0, len(validatorIDs))
		for _, validatorID := range validatorIDs {
			if validatorID >= uint64(len(state.Validators)) {
				return nil, nil, fmt.Errorf("validator index out of range: %d", validatorID)
			}

			pubkey := state.Validators[validatorID].Pubkey
			sa := group[validatorID]
			if sa == nil || sa.Message == nil {
				continue
			}

			signature := sa.Signature
			key, keyOK := makeSignatureKey(validatorID, data)
			if keyOK {
				if cached, ok := c.gossipSignatures[key]; ok && (!hasNonZeroSignature(signature) || cached.slot >= sa.Message.Slot) {
					signature = cached.signature
				}
			}
			if !hasNonZeroSignature(signature) {
				continue
			}

			signerIDs = append(signerIDs, validatorID)
			pubkeys = append(pubkeys, pubkey[:])
			sig := make([]byte, len(signature))
			copy(sig, signature[:])
			signatures = append(signatures, sig)
		}
		if len(signerIDs) == 0 {
			continue
		}

		bits = makeAggregationBits(signerIDs)
		proof := c.findReusableAggregatedProof(data, signerIDs, bits)
		if proof == nil {
			if !proverReady {
				leanmultisig.SetupProver()
				proverReady = true
			}
			buildStart := time.Now()
			proofData, err := leanmultisig.Aggregate(pubkeys, signatures, root, uint32(data.Slot))
			metrics.PQSigSignaturesBuildingTime.Observe(time.Since(buildStart).Seconds())
			if err != nil {
				return nil, nil, fmt.Errorf("aggregate signatures: %w", err)
			}
			proof = &types.AggregatedSignatureProof{
				Participants: append([]byte(nil), bits...),
				ProofData:    proofData,
			}
			for _, validatorID := range signerIDs {
				c.storeAggregatedPayloadLocked(validatorID, data, proof)
			}
			log.Info(
				"attestation aggregate proof built (leanMultisig)",
				"slot", data.Slot,
				"participants", len(signerIDs),
				"proof_size", fmt.Sprintf("%d bytes", len(proofData)),
			)
		} else {
			log.Info(
				"attestation aggregate proof reused (leanMultisig)",
				"slot", data.Slot,
				"participants", len(signerIDs),
				"proof_size", fmt.Sprintf("%d bytes", len(proof.ProofData)),
			)
		}

		aggregatedAttestations = append(aggregatedAttestations, &types.AggregatedAttestation{
			AggregationBits: bits,
			Data:            data,
		})
		attestationProofs = append(attestationProofs, proof)
		metrics.PQSigAggregatedSignaturesTotal.Inc()
		metrics.PQSigAttestationsInAggregatedTotal.Add(float64(len(signerIDs)))
	}

	return aggregatedAttestations, attestationProofs, nil
}

func makeAggregationBits(validatorIDs []uint64) []byte {
	if len(validatorIDs) == 0 {
		return []byte{0x01}
	}
	maxValidatorID := validatorIDs[len(validatorIDs)-1]
	bits := statetransition.MakeBitlist(maxValidatorID + 1)
	for _, validatorID := range validatorIDs {
		bits = statetransition.SetBit(bits, validatorID, true)
	}
	return bits
}

func hasNonZeroSignature(signature [types.XMSSSignatureSize]byte) bool {
	for _, b := range signature {
		if b != 0 {
			return true
		}
	}
	return false
}

func (c *Store) findReusableAggregatedProof(
	data *types.AttestationData,
	validatorIDs []uint64,
	participants []byte,
) *types.AggregatedSignatureProof {
	if data == nil || len(validatorIDs) == 0 {
		return nil
	}

	firstKey, ok := makeSignatureKey(validatorIDs[0], data)
	if !ok {
		return nil
	}
	candidates := c.aggregatedPayloads[firstKey]

	for _, candidate := range candidates {
		if candidate.proof == nil {
			continue
		}
		if !bitlistsEqual(candidate.proof.Participants, participants) {
			continue
		}

		matchAll := true
		for _, validatorID := range validatorIDs[1:] {
			key, keyOK := makeSignatureKey(validatorID, data)
			if !keyOK || !containsCachedProof(c.aggregatedPayloads[key], candidate.proof) {
				matchAll = false
				break
			}
		}
		if matchAll {
			return cloneAggregatedSignatureProof(candidate.proof)
		}
	}

	return nil
}

func containsCachedProof(list []storedAggregatedPayload, target *types.AggregatedSignatureProof) bool {
	for _, candidate := range list {
		if candidate.proof == nil {
			continue
		}
		if !bitlistsEqual(candidate.proof.Participants, target.Participants) {
			continue
		}
		if bytes.Equal(candidate.proof.ProofData, target.ProofData) {
			return true
		}
	}
	return false
}
