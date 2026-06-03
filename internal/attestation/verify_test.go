package attestation_test

import (
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/types"
)

// stateWithValidators builds a minimal head state with n zero-valued validators,
// used by the signature-verification tests to resolve attester pubkeys.
func stateWithValidators(slot uint64, n int) *types.State {
	validators := make([]*types.Validator, n)
	for i := range validators {
		validators[i] = &types.Validator{Index: uint64(i)}
	}
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{Slot: slot},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestVerifyGossipAttestationTargetStateMissing(t *testing.T) {
	s := makeValidationStore()
	data := makeValidAttestationData()
	var dataRoot [32]byte

	err := attestation.VerifyGossipAttestation(s, 0, data, dataRoot, make([]byte, types.SignatureSize))
	if err == nil || !strings.Contains(err.Error(), "target state not found") {
		t.Fatalf("error=%v, want target state missing", err)
	}
}

func TestVerifyGossipAttestationBadSignatureLength(t *testing.T) {
	s := makeValidationStore()
	data := makeValidAttestationData()
	s.InsertState(data.Target.Root, stateWithValidators(data.Target.Slot, 1))
	var dataRoot [32]byte

	err := attestation.VerifyGossipAttestation(s, 0, data, dataRoot, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "signature length") {
		t.Fatalf("error=%v, want signature length error", err)
	}
}

func TestVerifyAggregatedGossipAttestationParticipantOutOfRange(t *testing.T) {
	s := makeValidationStore()
	data := makeValidAttestationData()
	s.InsertState(data.Target.Root, stateWithValidators(data.Target.Slot, 1))
	participants := types.NewBitlistSSZ(3)
	types.BitlistSet(participants, 2)

	err := attestation.VerifyAggregatedGossipAttestation(s, data, participants, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("error=%v, want participant out-of-range error", err)
	}
}
