package xmss

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// TestKeyManagerDualKeyRouting verifies that SignAttestation uses the attestation
// key and SignBlock uses the proposal key, and they produce different signatures.
func TestKeyManagerDualKeyRouting(t *testing.T) {
	// Generate two independent keypairs for validator 0.
	attKp, err := GenerateKeyPair("test-attestation-key-0", 0, 1<<16)
	if err != nil {
		t.Fatalf("generate attestation keypair: %v", err)
	}
	defer attKp.Close()

	propKp, err := GenerateKeyPair("test-proposal-key-0", 0, 1<<16)
	if err != nil {
		t.Fatalf("generate proposal keypair: %v", err)
	}
	defer propKp.Close()

	// Verify keys are different.
	attPk, _ := attKp.PublicKeyBytes()
	propPk, _ := propKp.PublicKeyBytes()
	if attPk == propPk {
		t.Fatal("attestation and proposal keys should be different")
	}

	// Build KeyManager with dual keys.
	attestationKeys := map[uint64]*ValidatorKeyPair{0: attKp}
	proposalKeys := map[uint64]*ValidatorKeyPair{0: propKp}
	km := NewKeyManager(attestationKeys, proposalKeys)

	// Verify accessor routing.
	if km.GetAttestationKey(0) != attKp {
		t.Fatal("GetAttestationKey returned wrong key")
	}
	if km.GetProposalKey(0) != propKp {
		t.Fatal("GetProposalKey returned wrong key")
	}
	if km.GetAttestationKey(99) != nil {
		t.Fatal("GetAttestationKey should return nil for unknown validator")
	}
	if km.GetProposalKey(99) != nil {
		t.Fatal("GetProposalKey should return nil for unknown validator")
	}

	// Verify ValidatorIDs.
	ids := km.ValidatorIDs()
	if len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("ValidatorIDs: expected [0], got %v", ids)
	}

	// Sign attestation data — should use attestation key.
	attData := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: [32]byte{1}, Slot: 1},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 1},
		Source: &types.Checkpoint{Root: [32]byte{3}, Slot: 0},
	}
	attSig, err := km.SignAttestation(0, attData)
	if err != nil {
		t.Fatalf("SignAttestation: %v", err)
	}
	if attSig == [types.SignatureSize]byte{} {
		t.Fatal("SignAttestation produced zero signature")
	}

	// Sign block root — should use proposal key.
	blockRoot := [32]byte{0xaa, 0xbb}
	blockSig, err := km.SignBlock(0, 1, blockRoot)
	if err != nil {
		t.Fatalf("SignBlock: %v", err)
	}
	if blockSig == [types.SignatureSize]byte{} {
		t.Fatal("SignBlock produced zero signature")
	}

	// Signatures should be different (different keys, different messages).
	if attSig == blockSig {
		t.Fatal("attestation and block signatures should be different")
	}

	// Verify attestation sig with attestation pubkey.
	dataRoot, _ := attData.HashTreeRoot()
	valid, err := VerifySignatureSSZ(attPk, 1, dataRoot, attSig)
	if err != nil {
		t.Fatalf("verify attestation sig: %v", err)
	}
	if !valid {
		t.Fatal("attestation signature should verify with attestation key")
	}

	// Verify block sig with proposal pubkey.
	valid, err = VerifySignatureSSZ(propPk, 1, blockRoot, blockSig)
	if err != nil {
		t.Fatalf("verify block sig: %v", err)
	}
	if !valid {
		t.Fatal("block signature should verify with proposal key")
	}

	// Cross-verify should fail: attestation sig with proposal key.
	valid, _ = VerifySignatureSSZ(propPk, 1, dataRoot, attSig)
	if valid {
		t.Fatal("attestation signature should NOT verify with proposal key")
	}

	// Cross-verify should fail: block sig with attestation key.
	valid, _ = VerifySignatureSSZ(attPk, 1, blockRoot, blockSig)
	if valid {
		t.Fatal("block signature should NOT verify with attestation key")
	}
}

// TestKeyManagerSignErrors verifies error cases.
func TestKeyManagerSignErrors(t *testing.T) {
	km := NewKeyManager(
		map[uint64]*ValidatorKeyPair{},
		map[uint64]*ValidatorKeyPair{},
	)

	// SignAttestation with unknown validator.
	attData := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: [32]byte{1}, Slot: 1},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 1},
		Source: &types.Checkpoint{Root: [32]byte{3}, Slot: 0},
	}
	_, err := km.SignAttestation(99, attData)
	if err == nil {
		t.Fatal("SignAttestation should error for unknown validator")
	}

	// SignBlock with unknown validator.
	_, err = km.SignBlock(99, 1, [32]byte{})
	if err == nil {
		t.Fatal("SignBlock should error for unknown validator")
	}
}
