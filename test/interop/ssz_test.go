package interop

import (
	"encoding/hex"
	"testing"

	"github.com/geanlabs/gean/types"
)

// SSZ round-trip tests for devnet-1 types.

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
		ValidatorID: 2,
		Message: &types.AttestationData{
			Slot:   5,
			Head:   &types.Checkpoint{Root: headRoot, Slot: 3},
			Target: &types.Checkpoint{Root: targetRoot, Slot: 4},
			Source: &types.Checkpoint{Root: sourceRoot, Slot: 1},
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

	attRoot, err := sa.Message.HashTreeRoot()
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

func TestValidatorSSZWithFullPubkey(t *testing.T) {
	// 52-byte XMSS pubkey (all distinct bytes).
	var pubkey [52]byte
	for i := range pubkey {
		pubkey[i] = byte(i + 1)
	}
	v := &types.Validator{Pubkey: pubkey, Index: 99}

	data, err := v.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	// Validator is fixed-size: 52 (pubkey) + 8 (index) = 60 bytes.
	if len(data) != 60 {
		t.Fatalf("expected 60 SSZ bytes, got %d", len(data))
	}

	decoded := new(types.Validator)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if decoded.Pubkey != pubkey {
		t.Fatal("pubkey mismatch after round-trip")
	}
	if decoded.Index != 99 {
		t.Fatalf("index = %d, want 99", decoded.Index)
	}

	// Hash root stability.
	root1, _ := v.HashTreeRoot()
	root2, _ := decoded.HashTreeRoot()
	if root1 != root2 {
		t.Errorf("hash root changed after round-trip: %x != %x", root1, root2)
	}
}

func TestSignedBlockWithAttestationFullSSZRoundTrip(t *testing.T) {
	// Build a SignedBlockWithAttestation with a ProposerAttestation and signatures.
	proposerAtt := &types.Attestation{
		ValidatorID: 0,
		Data: &types.AttestationData{
			Slot:   1,
			Head:   &types.Checkpoint{Root: [32]byte{0x11}, Slot: 1},
			Target: &types.Checkpoint{Root: [32]byte{0x22}, Slot: 1},
			Source: &types.Checkpoint{Root: [32]byte{0x33}, Slot: 0},
		},
	}

	blockAtt := &types.Attestation{
		ValidatorID: 1,
		Data: &types.AttestationData{
			Slot:   0,
			Head:   &types.Checkpoint{Root: [32]byte{0x44}, Slot: 0},
			Target: &types.Checkpoint{Root: [32]byte{0x44}, Slot: 0},
			Source: &types.Checkpoint{Root: [32]byte{0x55}, Slot: 0},
		},
	}

	// Two signatures: one for the block attestation, one for the proposer attestation.
	var sig1, sig2 [3112]byte
	sig1[0] = 0xaa
	sig1[3111] = 0xbb
	sig2[0] = 0xcc

	sb := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{
				Slot:          1,
				ProposerIndex: 0,
				ParentRoot:    [32]byte{0xab},
				StateRoot:     [32]byte{0xcd},
				Body:          &types.BlockBody{Attestations: []*types.Attestation{blockAtt}},
			},
			ProposerAttestation: proposerAtt,
		},
		Signature: [][3112]byte{sig1, sig2},
	}

	data, err := sb.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	t.Logf("Full SignedBlockWithAttestation SSZ: %d bytes", len(data))

	decoded := new(types.SignedBlockWithAttestation)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}

	// Verify block fields.
	if decoded.Message.Block.Slot != 1 || decoded.Message.Block.ProposerIndex != 0 {
		t.Fatal("block fields mismatch")
	}

	// Verify proposer attestation.
	if decoded.Message.ProposerAttestation == nil {
		t.Fatal("ProposerAttestation should not be nil")
	}
	if decoded.Message.ProposerAttestation.ValidatorID != 0 {
		t.Fatalf("ProposerAttestation.ValidatorID = %d, want 0", decoded.Message.ProposerAttestation.ValidatorID)
	}
	if decoded.Message.ProposerAttestation.Data.Slot != 1 {
		t.Fatalf("ProposerAttestation.Data.Slot = %d, want 1", decoded.Message.ProposerAttestation.Data.Slot)
	}

	// Verify signatures.
	if len(decoded.Signature) != 2 {
		t.Fatalf("expected 2 signatures, got %d", len(decoded.Signature))
	}
	if decoded.Signature[0][0] != 0xaa || decoded.Signature[0][3111] != 0xbb {
		t.Fatal("signature[0] data mismatch")
	}
	if decoded.Signature[1][0] != 0xcc {
		t.Fatal("signature[1] data mismatch")
	}

	// Verify block body attestation.
	if len(decoded.Message.Block.Body.Attestations) != 1 {
		t.Fatalf("expected 1 block attestation, got %d", len(decoded.Message.Block.Body.Attestations))
	}
	if decoded.Message.Block.Body.Attestations[0].ValidatorID != 1 {
		t.Fatal("block attestation validator mismatch")
	}

	// Hash root stability.
	root1, _ := sb.HashTreeRoot()
	root2, _ := decoded.HashTreeRoot()
	if root1 != root2 {
		t.Errorf("hash root changed after round-trip: %x != %x", root1, root2)
	}
}
