package forkchoice

import (
	"fmt"
	"sort"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

// AggregateAttestations collects attestations for the same data and
// concatenates their XMSS signatures in ascending validator index order.
func AggregateAttestations(attestations []*types.SignedAttestation) (*types.AggregatedAttestation, error) {
	if len(attestations) == 0 {
		return nil, fmt.Errorf("no attestations to aggregate")
	}

	sorted := make([]*types.SignedAttestation, len(attestations))
	copy(sorted, attestations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ValidatorID < sorted[j].ValidatorID
	})

	maxID := sorted[len(sorted)-1].ValidatorID
	bits := statetransition.MakeBitlist(maxID + 1)
	for _, sa := range sorted {
		bits = statetransition.SetBit(bits, sa.ValidatorID, true)
	}

	aggSig := make([]byte, 0, len(sorted)*types.XMSSSignatureSize)
	for _, sa := range sorted {
		aggSig = append(aggSig, sa.Signature[:]...)
	}

	return &types.AggregatedAttestation{
		Data:                sorted[0].Message,
		AggregationBits:     bits,
		AggregatedSignature: aggSig,
	}, nil
}

// DisaggregateAttestation splits an aggregated attestation back into
// individual validator-signature pairs.
func DisaggregateAttestation(agg *types.AggregatedAttestation) ([]uint64, [][types.XMSSSignatureSize]byte, error) {
	numBits := uint64(statetransition.BitlistLen(agg.AggregationBits))
	var validatorIDs []uint64
	for i := uint64(0); i < numBits; i++ {
		if statetransition.GetBit(agg.AggregationBits, i) {
			validatorIDs = append(validatorIDs, i)
		}
	}

	expectedLen := len(validatorIDs) * types.XMSSSignatureSize
	if len(agg.AggregatedSignature) != expectedLen {
		return nil, nil, fmt.Errorf(
			"signature length mismatch: got %d, expected %d (%d validators × %d bytes)",
			len(agg.AggregatedSignature), expectedLen, len(validatorIDs), types.XMSSSignatureSize,
		)
	}

	sigs := make([][types.XMSSSignatureSize]byte, len(validatorIDs))
	for i := range validatorIDs {
		copy(sigs[i][:], agg.AggregatedSignature[i*types.XMSSSignatureSize:(i+1)*types.XMSSSignatureSize])
	}

	return validatorIDs, sigs, nil
}

// VerifyAggregatedAttestation disaggregates and verifies each XMSS signature.
// Returns the count of valid signatures.
func VerifyAggregatedAttestation(state *types.State, agg *types.AggregatedAttestation) (int, error) {
	validatorIDs, sigs, err := DisaggregateAttestation(agg)
	if err != nil {
		return 0, fmt.Errorf("disaggregate: %w", err)
	}
	verified := 0

	for i, valID := range validatorIDs {
		if valID >= uint64(len(state.Validators)) {
			log.Warn("aggregated attestation: invalid validator index", "validator", valID)
			continue
		}
		pubkey := state.Validators[valID].Pubkey
		att := &types.Attestation{ValidatorID: valID, Data: agg.Data}
		messageRoot, err := att.HashTreeRoot()
		if err != nil {
			return 0, fmt.Errorf("hash attestation: %w", err)
		}
		if err := leansig.Verify(pubkey[:], uint32(agg.Data.Slot), messageRoot, sigs[i][:]); err != nil {
			log.Warn("aggregated attestation: signature invalid",
				"validator", valID, "slot", agg.Data.Slot, "err", err,
			)
			continue
		}
		verified++
	}

	return verified, nil
}

// ProcessAggregatedAttestation validates and counts votes from an aggregate.
func (c *Store) ProcessAggregatedAttestation(agg *types.AggregatedAttestation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.validateAttestationData(agg.Data) {
		return
	}

	headState, ok := c.storage.GetState(c.head)
	if !ok {
		return
	}

	validatorIDs, sigs, err := DisaggregateAttestation(agg)
	if err != nil {
		log.Warn("disaggregate failed", "err", err)
		return
	}

	currentSlot := c.time / types.IntervalsPerSlot

	for i, valID := range validatorIDs {
		if valID >= uint64(len(headState.Validators)) {
			continue
		}
		pubkey := headState.Validators[valID].Pubkey
		att := &types.Attestation{ValidatorID: valID, Data: agg.Data}
		messageRoot, err := att.HashTreeRoot()
		if err != nil {
			return
		}
		if err := leansig.Verify(pubkey[:], uint32(agg.Data.Slot), messageRoot, sigs[i][:]); err != nil {
			continue
		}
		if agg.Data.Slot > currentSlot {
			continue
		}

		sa := &types.SignedAttestation{
			ValidatorID: valID,
			Message:     agg.Data,
			Signature:   sigs[i],
		}
		existing, ok := c.latestNewAttestations[valID]
		if !ok || existing.Message.Slot < agg.Data.Slot {
			c.latestNewAttestations[valID] = sa
		}
	}
}
