package forkchoice

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leanmultisig"
)

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
	maxLen := statetransition.BitlistLen(a)
	if otherLen := statetransition.BitlistLen(b); otherLen > maxLen {
		maxLen = otherLen
	}
	for i := 0; i < maxLen; i++ {
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

	leanmultisig.SetupProver()

	aggregatedAttestations := make([]*types.AggregatedAttestation, 0, len(roots))
	attestationProofs := make([]*types.AggregatedSignatureProof, 0, len(roots))
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

		maxValidatorID := validatorIDs[len(validatorIDs)-1]
		bits := statetransition.MakeBitlist(maxValidatorID + 1)
		pubkeys := make([][]byte, 0, len(validatorIDs))
		signatures := make([][]byte, 0, len(validatorIDs))
		for _, validatorID := range validatorIDs {
			if validatorID >= uint64(len(state.Validators)) {
				return nil, nil, fmt.Errorf("validator index out of range: %d", validatorID)
			}
			bits = statetransition.SetBit(bits, validatorID, true)

			pubkey := state.Validators[validatorID].Pubkey
			pubkeys = append(pubkeys, pubkey[:])

			sa := group[validatorID]
			sig := make([]byte, len(sa.Signature))
			copy(sig, sa.Signature[:])
			signatures = append(signatures, sig)
		}

		proofData, err := leanmultisig.Aggregate(pubkeys, signatures, root, uint32(dataByRoot[root].Slot))
		if err != nil {
			return nil, nil, fmt.Errorf("aggregate signatures: %w", err)
		}

		aggregatedAttestations = append(aggregatedAttestations, &types.AggregatedAttestation{
			AggregationBits: bits,
			Data:            dataByRoot[root],
		})
		attestationProofs = append(attestationProofs, &types.AggregatedSignatureProof{
			Participants: append([]byte(nil), bits...),
			ProofData:    proofData,
		})
	}

	return aggregatedAttestations, attestationProofs, nil
}
