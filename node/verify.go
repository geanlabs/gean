package node

import (
	"fmt"

	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// verifyAttestation verifies a single XMSS signature.
func verifyAttestation(pubkey [types.PubkeySize]byte, slot uint32, message [32]byte, sig [types.SignatureSize]byte) (bool, error) {
	return xmss.VerifySignatureSSZ(pubkey, slot, message, sig)
}

// verifyAggregatedProof verifies an aggregated XMSS proof against participant pubkeys.
func verifyAggregatedProof(
	state *types.State,
	participantIDs []uint64,
	data *types.AttestationData,
	proofData []byte,
) error {
	numValidators := uint64(len(state.Validators))

	// Parse pubkeys for participants.
	parsedPubkeys := make([]xmss.CPubKey, len(participantIDs))
	for i, vid := range participantIDs {
		if vid >= numValidators {
			return fmt.Errorf("validator %d out of range (%d)", vid, numValidators)
		}
		pk, err := xmss.ParsePublicKey(state.Validators[vid].AttestationPubkey)
		if err != nil {
			// Free already parsed keys.
			for j := 0; j < i; j++ {
				xmss.FreePublicKey(parsedPubkeys[j])
			}
			return fmt.Errorf("parse pubkey for validator %d: %w", vid, err)
		}
		parsedPubkeys[i] = pk
	}
	defer func() {
		for _, pk := range parsedPubkeys {
			xmss.FreePublicKey(pk)
		}
	}()

	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash tree root: %w", err)
	}

	slot := uint32(data.Slot)
	return xmss.VerifyAggregatedSignature(proofData, parsedPubkeys, dataRoot, slot)
}
