package interop

import (
	"encoding/hex"
	"testing"

	"github.com/geanlabs/gean/types"
)

// SSZ round-trip tests for devnet-1 types.
// TODO: Add cross-client reference values from leanSpec devnet-1.

func TestSignedBlockWithAttestationSSZRoundTrip(t *testing.T) {
	var parentRoot, stateRoot [32]byte
	for i := range parentRoot {
		parentRoot[i] = 0xab
		stateRoot[i] = 0xcd
	}

	sb := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{
				Slot:          1,
				ProposerIndex: 0,
				ParentRoot:    parentRoot,
				StateRoot:     stateRoot,
				Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
			},
		},
	}

	data, err := sb.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	t.Logf("SignedBlockWithAttestation SSZ (%d bytes): %s", len(data), hex.EncodeToString(data))

	root, err := sb.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	t.Logf("SignedBlockWithAttestation root: %s", hex.EncodeToString(root[:]))

	blockRoot, err := sb.Message.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("Block HashTreeRoot: %v", err)
	}
	t.Logf("Block root: %s", hex.EncodeToString(blockRoot[:]))

	// Round-trip: decode and re-encode.
	decoded := new(types.SignedBlockWithAttestation)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	reencoded, err := decoded.MarshalSSZ()
	if err != nil {
		t.Fatalf("re-MarshalSSZ: %v", err)
	}
	if hex.EncodeToString(reencoded) != hex.EncodeToString(data) {
		t.Error("SSZ round-trip produced different bytes")
	}

	// Hash root stability.
	decodedRoot, _ := decoded.HashTreeRoot()
	if root != decodedRoot {
		t.Errorf("hash root changed after round-trip: %x != %x", root, decodedRoot)
	}
}

func TestSignedAttestationSSZRoundTrip(t *testing.T) {
	var headRoot, targetRoot, sourceRoot [32]byte
	for i := 0; i < 32; i++ {
		headRoot[i] = 0x11
		targetRoot[i] = 0x22
		sourceRoot[i] = 0x33
	}

	sa := &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: 2,
			Data: &types.AttestationData{
				Slot:   5,
				Head:   &types.Checkpoint{Root: headRoot, Slot: 3},
				Target: &types.Checkpoint{Root: targetRoot, Slot: 4},
				Source: &types.Checkpoint{Root: sourceRoot, Slot: 1},
			},
		},
	}

	data, err := sa.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	t.Logf("SignedAttestation SSZ (%d bytes): %s", len(data), hex.EncodeToString(data))

	root, err := sa.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	t.Logf("SignedAttestation root: %s", hex.EncodeToString(root[:]))

	attRoot, err := sa.Message.Data.HashTreeRoot()
	if err != nil {
		t.Fatalf("AttestationData HashTreeRoot: %v", err)
	}
	t.Logf("AttestationData root: %s", hex.EncodeToString(attRoot[:]))

	// Round-trip.
	decoded := new(types.SignedAttestation)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	reencoded, err := decoded.MarshalSSZ()
	if err != nil {
		t.Fatalf("re-MarshalSSZ: %v", err)
	}
	if hex.EncodeToString(reencoded) != hex.EncodeToString(data) {
		t.Error("SSZ round-trip produced different bytes")
	}

	decodedRoot, _ := decoded.HashTreeRoot()
	if root != decodedRoot {
		t.Errorf("hash root changed after round-trip: %x != %x", root, decodedRoot)
	}
}

func TestAttestationDataSSZRoundTrip(t *testing.T) {
	ad := &types.AttestationData{
		Slot:   10,
		Head:   &types.Checkpoint{Root: [32]byte{0xaa}, Slot: 9},
		Target: &types.Checkpoint{Root: [32]byte{0xbb}, Slot: 8},
		Source: &types.Checkpoint{Root: [32]byte{0xcc}, Slot: 7},
	}
	data, err := ad.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}

	decoded := new(types.AttestationData)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if decoded.Slot != 10 || decoded.Head.Slot != 9 || decoded.Target.Slot != 8 || decoded.Source.Slot != 7 {
		t.Fatal("AttestationData round-trip failed")
	}
}

func TestValidatorSSZRoundTrip(t *testing.T) {
	v := &types.Validator{
		Pubkey: [52]byte{0x01, 0x02, 0x03},
		Index:  42,
	}
	data, err := v.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}

	decoded := new(types.Validator)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if decoded.Pubkey != v.Pubkey || decoded.Index != v.Index {
		t.Fatal("Validator round-trip failed")
	}
}
