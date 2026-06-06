package attestation

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func VerifyGossipAttestation(s *store.ConsensusStore, validatorID uint64, attData *types.AttestationData, dataRoot [32]byte, signature []byte) error {
	if err := validateDataShape(attData); err != nil {
		return err
	}

	targetState := s.GetState(attData.Target.Root)
	if targetState == nil {
		return fmt.Errorf("target state not found in store: 0x%x", attData.Target.Root)
	}
	if validatorID >= uint64(len(targetState.Validators)) {
		return fmt.Errorf("validator %d not found in state (registry size %d)",
			validatorID, len(targetState.Validators))
	}
	if len(signature) != types.SignatureSize {
		return fmt.Errorf("signature length %d != expected %d", len(signature), types.SignatureSize)
	}
	var sig [types.SignatureSize]byte
	copy(sig[:], signature)
	valid, err := xmss.VerifySignatureSSZ(
		targetState.Validators[validatorID].AttestationPubkey,
		uint32(attData.Slot),
		dataRoot,
		sig,
	)
	if err != nil {
		return fmt.Errorf("signature verification error: %w", err)
	}
	if !valid {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func VerifyAggregatedGossipAttestation(s *store.ConsensusStore, attData *types.AttestationData, participants []byte, proofData []byte) error {
	if err := validateDataShape(attData); err != nil {
		return err
	}

	targetState := s.GetState(attData.Target.Root)
	if targetState == nil {
		return fmt.Errorf("target state not found in store: 0x%x", attData.Target.Root)
	}
	participantIDs := types.BitlistIndices(participants)
	return verifyAggregatedProof(targetState, participantIDs, attData, proofData)
}

func verifyAggregatedProof(
	state *types.State,
	participantIDs []uint64,
	data *types.AttestationData,
	proofData []byte,
) error {
	numValidators := uint64(len(state.Validators))
	parsedPubkeys := make([]xmss.CPubKey, len(participantIDs))
	for i, vid := range participantIDs {
		if vid >= numValidators {
			return fmt.Errorf("validator %d out of range (%d)", vid, numValidators)
		}
		pk, err := xmss.ParsePublicKey(state.Validators[vid].AttestationPubkey)
		if err != nil {
			for j := range i {
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
	return xmss.VerifyAggregatedSignature(proofData, parsedPubkeys, dataRoot, uint32(data.Slot))
}
