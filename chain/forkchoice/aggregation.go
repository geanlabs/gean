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

// aggregationInput holds pre-collected data for a single aggregation group,
// extracted while holding the lock so the FFI call can run without it.
type aggregationInput struct {
	root       [32]byte
	data       *types.AttestationData
	bits       []byte
	signerIDs  []uint64
	pubkeys    [][]byte
	signatures [][]byte
	// cachedProof is non-nil if a reusable proof was found in the cache.
	cachedProof *types.AggregatedSignatureProof
}

// AggregateCommitteeSignatures collects gossip signatures, builds aggregated
// proofs, and returns SignedAggregatedAttestation objects ready for publishing.
// Called by aggregators at interval 2.
//
// The mutex is released during the expensive leanmultisig.Aggregate() FFI calls
// to prevent blocking block processing, attestation handling, and time advances.
// This matches zeam's pattern of using a separate signatures_mutex from the
// forkchoice lock (forkchoice.zig:308).
func (c *Store) AggregateCommitteeSignatures() ([]*types.SignedAggregatedAttestation, error) {
	// Phase 1: Collect inputs while holding the lock.
	inputs, err := c.collectAggregationInputs()
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	// Phase 2: Build proofs WITHOUT the lock — this is the expensive part.
	results, err := buildAggregationProofs(inputs)
	if err != nil {
		return nil, fmt.Errorf("build aggregated proofs: %w", err)
	}

	// Phase 3: Store results and build output while holding the lock.
	return c.storeAggregationResults(results)
}

// collectAggregationInputs extracts attestation data and signatures from the
// gossip cache while holding the lock. Clears consumed gossip signatures.
func (c *Store) collectAggregationInputs() ([]aggregationInput, error) {
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

	// Group by attestation data root, keep latest per validator.
	grouped := make(map[[32]byte]map[uint64]*types.SignedAttestation)
	dataByRoot := make(map[[32]byte]*types.AttestationData)
	for _, sa := range attestations {
		if sa == nil || sa.Message == nil {
			continue
		}
		dataRoot, err := sa.Message.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("hash attestation data: %w", err)
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
		return nil, nil
	}

	// Sort roots for deterministic ordering.
	roots := make([][32]byte, 0, len(grouped))
	for root := range grouped {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return bytes.Compare(roots[i][:], roots[j][:]) < 0
	})

	// Build inputs for each group, checking the proof cache while we have the lock.
	var inputs []aggregationInput
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

		// Check for a cached proof before collecting signatures.
		if cached := c.findReusableAggregatedProof(data, validatorIDs, bits); cached != nil {
			inputs = append(inputs, aggregationInput{
				root:        root,
				data:        data,
				bits:        bits,
				signerIDs:   validatorIDs,
				cachedProof: cached,
			})
			continue
		}

		// Collect pubkeys and signatures for this group.
		signerIDs := make([]uint64, 0, len(validatorIDs))
		pubkeys := make([][]byte, 0, len(validatorIDs))
		signatures := make([][]byte, 0, len(validatorIDs))
		for _, validatorID := range validatorIDs {
			if validatorID >= uint64(len(headState.Validators)) {
				return nil, fmt.Errorf("validator index out of range: %d", validatorID)
			}
			pubkey := headState.Validators[validatorID].Pubkey
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

		// Rebuild bits for actual signers (may differ from validatorIDs if some had zero sigs).
		bits = makeAggregationBits(signerIDs)

		// Check cache again with actual signer set.
		if cached := c.findReusableAggregatedProof(data, signerIDs, bits); cached != nil {
			inputs = append(inputs, aggregationInput{
				root:        root,
				data:        data,
				bits:        bits,
				signerIDs:   signerIDs,
				cachedProof: cached,
			})
			continue
		}

		inputs = append(inputs, aggregationInput{
			root:       root,
			data:       data,
			bits:       bits,
			signerIDs:  signerIDs,
			pubkeys:    pubkeys,
			signatures: signatures,
		})
	}

	// Clear consumed gossip signatures (spec: remove aggregated entries).
	c.gossipSignatures = make(map[signatureKey]storedSignature)

	return inputs, nil
}

// buildAggregationProofs runs the expensive leanmultisig.Aggregate() FFI calls
// WITHOUT holding the fork-choice mutex. This is the critical change that
// prevents consensus stalls during proof building.
func buildAggregationProofs(inputs []aggregationInput) ([]aggregationInput, error) {
	proverReady := false

	for i := range inputs {
		inp := &inputs[i]
		if inp.cachedProof != nil {
			log.Info("attestation aggregate proof reused (leanMultisig)",
				"slot", inp.data.Slot,
				"participants", len(inp.signerIDs),
				"proof_size", fmt.Sprintf("%d bytes", len(inp.cachedProof.ProofData)),
			)
			continue
		}

		if !proverReady {
			leanmultisig.SetupProver()
			proverReady = true
		}

		buildStart := time.Now()
		proofData, err := leanmultisig.Aggregate(inp.pubkeys, inp.signatures, inp.root, uint32(inp.data.Slot))
		metrics.PQSigSignaturesBuildingTime.Observe(time.Since(buildStart).Seconds())
		if err != nil {
			return nil, fmt.Errorf("aggregate signatures for slot %d: %w", inp.data.Slot, err)
		}

		inp.cachedProof = &types.AggregatedSignatureProof{
			Participants: append([]byte(nil), inp.bits...),
			ProofData:    proofData,
		}
		log.Info("attestation aggregate proof built (leanMultisig)",
			"slot", inp.data.Slot,
			"participants", len(inp.signerIDs),
			"proof_size", fmt.Sprintf("%d bytes", len(proofData)),
		)
	}

	return inputs, nil
}

// storeAggregationResults stores built proofs in the cache and assembles the
// final output while holding the lock.
func (c *Store) storeAggregationResults(inputs []aggregationInput) ([]*types.SignedAggregatedAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*types.SignedAggregatedAttestation, 0, len(inputs))
	for _, inp := range inputs {
		if inp.cachedProof == nil {
			continue
		}

		// Store in cache for future reuse.
		for _, validatorID := range inp.signerIDs {
			c.storeAggregatedPayloadLocked(validatorID, inp.data, inp.cachedProof)
		}

		result = append(result, &types.SignedAggregatedAttestation{
			Data:  inp.data,
			Proof: inp.cachedProof,
		})
		metrics.PQSigAggregatedSignaturesTotal.Inc()
		metrics.PQSigAttestationsInAggregatedTotal.Add(float64(len(inp.signerIDs)))
	}

	return result, nil
}

// buildAggregatedAttestationsFromSignedLocked builds aggregated attestations
// and proofs from a set of signed attestations. Must be called with c.mu held.
// Used by block production (produce.go) which already holds the lock and needs
// proofs synchronously before publishing the block.
func (c *Store) buildAggregatedAttestationsFromSignedLocked(
	state *types.State,
	attestations []*types.SignedAttestation,
) ([]*types.AggregatedAttestation, []*types.AggregatedSignatureProof, error) {
	if len(attestations) == 0 {
		return []*types.AggregatedAttestation{}, []*types.AggregatedSignatureProof{}, nil
	}

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
			proofData, err := leanmultisig.Aggregate(pubkeys, signatures, root, uint32(data.Slot))
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
