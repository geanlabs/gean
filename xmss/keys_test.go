package xmss

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestKeyManagerValidatorIDsSorted(t *testing.T) {
	km := NewKeyManager(map[uint64]*ValidatorKeyPair{
		7: nil,
		1: nil,
		3: nil,
	}, nil)

	ids := km.ValidatorIDs()
	want := []uint64{1, 3, 7}
	if len(ids) != len(want) {
		t.Fatalf("validator IDs=%v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("validator IDs=%v, want %v", ids, want)
		}
	}
}

func TestKeyManagerNilSafeAccessors(t *testing.T) {
	var km *KeyManager
	if ids := km.ValidatorIDs(); ids != nil {
		t.Fatalf("ValidatorIDs=%v, want nil", ids)
	}
	if km.GetAttestationKey(1) != nil {
		t.Fatal("nil key manager should not return attestation key")
	}
	if km.GetProposalKey(1) != nil {
		t.Fatal("nil key manager should not return proposal key")
	}
	km.Close()

	if _, err := km.SignAttestation(1, nil); err == nil || !strings.Contains(err.Error(), "key manager is nil") {
		t.Fatalf("SignAttestation error=%v, want nil key manager error", err)
	}
	if _, err := km.SignBlock(1, 0, [32]byte{}); err == nil || !strings.Contains(err.Error(), "key manager is nil") {
		t.Fatalf("SignBlock error=%v, want nil key manager error", err)
	}
}

// writeKeyFiles generates one key pair per name and writes its secret key under that name in dir.
func writeKeyFiles(t *testing.T, dir string, names ...string) map[string][types.PubkeySize]byte {
	t.Helper()
	pubkeys := make(map[string][types.PubkeySize]byte, len(names))
	for _, name := range names {
		kp, err := GenerateKeyPair("keys-test-"+name, 0, 1<<10)
		if err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
		sk, err := kp.PrivateKeyBytes()
		if err != nil {
			t.Fatalf("serialize %s: %v", name, err)
		}
		if pubkeys[name], err = kp.PublicKeyBytes(); err != nil {
			t.Fatalf("pubkey %s: %v", name, err)
		}
		kp.Close()
		if err := os.WriteFile(filepath.Join(dir, name), sk, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return pubkeys
}

func loadAnnotated(t *testing.T, dir, entries string) (*KeyManager, error) {
	t.Helper()
	path := filepath.Join(dir, "validators.yaml")
	if err := os.WriteFile(path, []byte("node-a:\n"+entries), 0o600); err != nil {
		t.Fatalf("write annotated validators: %v", err)
	}
	return LoadValidatorKeys(path, dir, "node-a")
}

func TestLoadValidatorKeys(t *testing.T) {
	dir := t.TempDir()
	writeKeyFiles(t, dir, "v_attester_key_sk.ssz", "v_proposer_key_sk.ssz", "v_sk.ssz", "v_attester_proposer_sk.ssz")

	tests := []struct {
		name    string
		entries string
		wantErr string
	}{
		{"lean-quickstart pair", `
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
  - {index: 0, privkey_file: v_proposer_key_sk.ssz}
`, ""},
		{"keygen pair", `
  - {index: 0, attestation_sk_file: v_attester_key_sk.ssz, proposal_sk_file: v_proposer_key_sk.ssz}
`, ""},
		{"file without role", `
  - {index: 0, privkey_file: v_sk.ssz}
`, "must name exactly one role"},
		{"file naming both roles", `
  - {index: 0, privkey_file: v_attester_proposer_sk.ssz}
  - {index: 0, privkey_file: v_proposer_key_sk.ssz}
`, "must name exactly one role"},
		{"duplicate role", `
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
  - {index: 0, privkey_file: v_proposer_key_sk.ssz}
`, "duplicate attestation key for validator 0"},
		{"missing proposal key", `
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
`, "proposal key file missing for validator 0"},
		{"incomplete keygen pair", `
  - {index: 0, attestation_sk_file: v_attester_key_sk.ssz}
`, "proposal key file missing for validator 0"},
		{"mixed formats", `
  - {index: 0, privkey_file: v_attester_key_sk.ssz, proposal_sk_file: v_proposer_key_sk.ssz}
`, "cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km, err := loadAnnotated(t, dir, tt.entries)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("load: %v", err)
				}
				km.Close()
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				km.Close()
				t.Fatalf("error=%v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestKeyManagerMatchRegistry(t *testing.T) {
	dir := t.TempDir()
	pubkeys := writeKeyFiles(t, dir, "v_attester_key_sk.ssz", "v_proposer_key_sk.ssz")
	att, prop := pubkeys["v_attester_key_sk.ssz"], pubkeys["v_proposer_key_sk.ssz"]

	// The attester file copied under a proposer name: one key in both roles.
	sk, err := os.ReadFile(filepath.Join(dir, "v_attester_key_sk.ssz"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "copy_proposer_key_sk.ssz"), sk, 0o600); err != nil {
		t.Fatal(err)
	}

	pair := `
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
  - {index: 0, privkey_file: v_proposer_key_sk.ssz}
`
	tests := []struct {
		name     string
		entries  string
		registry []*types.Validator
		wantErr  string
	}{
		{"matching keys", pair, []*types.Validator{{AttestationPubkey: att, ProposalPubkey: prop}}, ""},
		{"swapped keys", pair, []*types.Validator{{AttestationPubkey: prop, ProposalPubkey: att}}, "attestation key"},
		{"one key in both roles", `
  - {index: 0, privkey_file: v_attester_key_sk.ssz}
  - {index: 0, privkey_file: copy_proposer_key_sk.ssz}
`, []*types.Validator{{AttestationPubkey: att, ProposalPubkey: prop}}, "proposal key"},
		{"validator outside registry", `
  - {index: 1, privkey_file: v_attester_key_sk.ssz}
  - {index: 1, privkey_file: v_proposer_key_sk.ssz}
`, []*types.Validator{{AttestationPubkey: att, ProposalPubkey: prop}}, "not in registry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km, err := loadAnnotated(t, dir, tt.entries)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			defer km.Close()
			err = km.MatchRegistry(tt.registry)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("match: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadKeypairRejectsEmptySecretKey(t *testing.T) {
	dir := t.TempDir()
	skPath := filepath.Join(dir, "empty.sk")
	if err := os.WriteFile(skPath, nil, 0o600); err != nil {
		t.Fatalf("write secret key: %v", err)
	}

	if _, err := loadKeypair(dir, "empty.sk", 0); err == nil || !strings.Contains(err.Error(), "secret key is empty") {
		t.Fatalf("loadKeypair error=%v, want empty secret key rejection", err)
	}
}

func TestKeyManagerDualKeyRouting(t *testing.T) {
	attKp, err := GenerateKeyPair("test-attestation-key-0", 0, 1<<10)
	if err != nil {
		t.Fatalf("generate attestation keypair: %v", err)
	}
	defer attKp.Close()

	propKp, err := GenerateKeyPair("test-proposal-key-0", 0, 1<<10)
	if err != nil {
		t.Fatalf("generate proposal keypair: %v", err)
	}
	defer propKp.Close()

	attPk, err := attKp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("attestation pubkey: %v", err)
	}
	propPk, err := propKp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("proposal pubkey: %v", err)
	}
	if attPk == propPk {
		t.Fatal("attestation and proposal keys should be different")
	}

	attestationKeys := map[uint64]*ValidatorKeyPair{0: attKp}
	proposalKeys := map[uint64]*ValidatorKeyPair{0: propKp}
	km := NewKeyManager(attestationKeys, proposalKeys)

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

	ids := km.ValidatorIDs()
	if len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("ValidatorIDs: expected [0], got %v", ids)
	}

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

	blockRoot := [32]byte{0xaa, 0xbb}
	blockSig, err := km.SignBlock(0, 1, blockRoot)
	if err != nil {
		t.Fatalf("SignBlock: %v", err)
	}
	if blockSig == [types.SignatureSize]byte{} {
		t.Fatal("SignBlock produced zero signature")
	}

	if attSig == blockSig {
		t.Fatal("attestation and block signatures should be different")
	}

	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		t.Fatalf("attestation data root: %v", err)
	}
	valid, err := VerifySignatureSSZ(attPk, 1, dataRoot, attSig)
	if err != nil {
		t.Fatalf("verify attestation sig: %v", err)
	}
	if !valid {
		t.Fatal("attestation signature should verify with attestation key")
	}

	valid, err = VerifySignatureSSZ(propPk, 1, blockRoot, blockSig)
	if err != nil {
		t.Fatalf("verify block sig: %v", err)
	}
	if !valid {
		t.Fatal("block signature should verify with proposal key")
	}

	valid, err = VerifySignatureSSZ(propPk, 1, dataRoot, attSig)
	if err != nil {
		t.Fatalf("cross verify attestation sig: %v", err)
	}
	if valid {
		t.Fatal("attestation signature should NOT verify with proposal key")
	}

	valid, err = VerifySignatureSSZ(attPk, 1, blockRoot, blockSig)
	if err != nil {
		t.Fatalf("cross verify block sig: %v", err)
	}
	if valid {
		t.Fatal("block signature should NOT verify with attestation key")
	}
}

func TestValidatorKeyPairRejectsClosedKey(t *testing.T) {
	kp, err := GenerateKeyPair("test-closed-keypair", 0, 1<<10)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	kp.Close()

	if kp.PublicKeyPtr() != nil {
		t.Fatal("closed keypair should not expose public key pointer")
	}
	if kp.PrivateKeyPtr() != nil {
		t.Fatal("closed keypair should not expose private key pointer")
	}
	if _, err := kp.Sign(0, [32]byte{}); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("Sign error=%v, want closed keypair error", err)
	}
	if _, err := kp.PublicKeyBytes(); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("PublicKeyBytes error=%v, want closed keypair error", err)
	}
	if _, err := kp.PrivateKeyBytes(); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("PrivateKeyBytes error=%v, want closed keypair error", err)
	}
}

func TestKeyManagerSignErrors(t *testing.T) {
	km := NewKeyManager(
		map[uint64]*ValidatorKeyPair{},
		map[uint64]*ValidatorKeyPair{},
	)

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

	_, err = km.SignBlock(99, 1, [32]byte{})
	if err == nil {
		t.Fatal("SignBlock should error for unknown validator")
	}
}
