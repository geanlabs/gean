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
//
// Proof building (leanMultisig.Aggregate) is CPU-intensive (1-4 seconds).
// To avoid blocking gossip message processing, the mutex is released during
// proof building and re-acquired to store results.
func (c *Store) AggregateCommitteeSignatures() ([]*types.SignedAggregatedAttestation, error) {
	// Phase 1: Collect gossip signatures under lock.
	c.mu.Lock()
	headState, ok := c.storage.GetState(c.head)
	if !ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("head state not found")
	}
	currentSlot := c.time / types.IntervalsPerSlot

	var attestations []*types.SignedAttestation
	deferred := make(map[signatureKey]storedSignature)
	droppedStale := 0
	deferredFuture := 0
	for key, stored := range c.gossipSignatures {
		if stored.data == nil {
			continue
		}
		switch {
		case stored.data.Slot < currentSlot:
			droppedStale++
			continue
		case stored.data.Slot > currentSlot:
			deferred[key] = stored
			deferredFuture++
			continue
		}
		attestations = append(attestations, &types.SignedAttestation{
			ValidatorID: key.validatorID,
			Message:     stored.data,
			Signature:   stored.signature,
		})
	}

	c.gossipSignatures = deferred
	metrics.GossipSignaturesCount.Set(float64(len(c.gossipSignatures)))

	if len(attestations) == 0 {
		c.mu.Unlock()
		if droppedStale > 0 || deferredFuture > 0 {
			log.Info("aggregation backlog filtered",
				"current_slot", currentSlot,
				"eligible_signatures", 0,
				"dropped_stale_signatures", droppedStale,
				"deferred_future_signatures", deferredFuture,
				"built_aggregates", 0,
			)
		}
		return nil, nil
	}

	c.mu.Unlock()

	// Phase 2: Build proofs WITHOUT the lock. This is the expensive part
	// (1-4 seconds per proof). Other goroutines can store new gossip
	// signatures during this time.
	aggAtts, aggProofs, err := c.buildAggregatedProofs(headState, attestations, false)
	if err != nil {
		return nil, fmt.Errorf("build aggregated attestations: %w", err)
	}

	// Phase 3: Store results under lock.
	c.mu.Lock()
	for i, att := range aggAtts {
		if i >= len(aggProofs) || aggProofs[i] == nil {
			continue
		}
		for _, vid := range bitlistToValidatorIDs(att.AggregationBits) {
			c.storeAggregatedPayloadLocked(vid, att.Data, aggProofs[i])
		}
	}
	c.mu.Unlock()

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

	if droppedStale > 0 || deferredFuture > 0 || len(result) > 0 {
		log.Info("aggregation backlog filtered",
			"current_slot", currentSlot,
			"eligible_signatures", len(attestations),
			"dropped_stale_signatures", droppedStale,
			"deferred_future_signatures", deferredFuture,
			"built_aggregates", len(result),
		)
	}

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

// buildAggregatedAttestationsFromSigned builds aggregated attestations and proofs
// from signed attestations. Called under lock from ProduceBlock (block building).
func (c *Store) buildAggregatedAttestationsFromSigned(
	state *types.State,
	attestations []*types.SignedAttestation,
) ([]*types.AggregatedAttestation, []*types.AggregatedSignatureProof, error) {
	atts, proofs, err := c.buildAggregatedProofs(state, attestations, true)
	if err != nil {
		return nil, nil, err
	}
	// Store proofs for reuse.
	for i, att := range atts {
		if i >= len(proofs) || proofs[i] == nil {
			continue
		}
		for _, vid := range bitlistToValidatorIDs(att.AggregationBits) {
			c.storeAggregatedPayloadLocked(vid, att.Data, proofs[i])
		}
	}
	return atts, proofs, nil
}

// buildAggregatedProofs builds aggregated proofs. When useCachedSigs is true,
// it falls back to gossipSignatures for missing signatures (requires lock).
// When false, it only uses signatures from the provided attestations.
func (c *Store) buildAggregatedProofs(
	state *types.State,
	attestations []*types.SignedAttestation,
	useCachedSigs bool,
) ([]*types.AggregatedAttestation, []*types.AggregatedSignatureProof, error) {
	if len(attestations) == 0 {
		return []*types.AggregatedAttestation{}, []*types.AggregatedSignatureProof{}, nil
	}

	// Group by attestation data root.
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

		signerIDs := make([]uint64, 0, len(validatorIDs))
		pubkeys := make([][]byte, 0, len(validatorIDs))
		signatures := make([][]byte, 0, len(validatorIDs))
		for _, validatorID := range validatorIDs {
			if validatorID >= uint64(len(state.Validators)) {
				return nil, nil, fmt.Errorf("validator index out of range: %d", validatorID)
			}
			sa := group[validatorID]
			if sa == nil || sa.Message == nil {
				continue
			}
			signature := sa.Signature
			// When building blocks (useCachedSigs=true), fall back to stored
			// gossip signatures for validators whose attestation has no sig.
			if useCachedSigs {
				key, keyOK := makeSignatureKey(validatorID, data)
				if keyOK {
					if cached, ok := c.gossipSignatures[key]; ok && (!hasNonZeroSignature(signature) || cached.slot >= sa.Message.Slot) {
						signature = cached.signature
					}
				}
			}
			if !hasNonZeroSignature(signature) {
				continue
			}
			signerIDs = append(signerIDs, validatorID)
			pubkey := state.Validators[validatorID].Pubkey
			pubkeys = append(pubkeys, pubkey[:])
			sig := make([]byte, len(signature))
			copy(sig, signature[:])
			signatures = append(signatures, sig)
		}
		if len(signerIDs) == 0 {
			continue
		}

		bits := makeAggregationBits(signerIDs)
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
		proof := &types.AggregatedSignatureProof{
			Participants: append([]byte(nil), bits...),
			ProofData:    proofData,
		}
		log.Info(
			"attestation aggregate proof built (leanMultisig)",
			"slot", data.Slot,
			"participants", len(signerIDs),
			"proof_size", fmt.Sprintf("%d bytes", len(proofData)),
		)

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
