package xmss

// #include <stdint.h>
// #include <stdlib.h>
// typedef struct KeyPair KeyPair;
// typedef struct PublicKey PublicKey;
// typedef struct PrivateKey PrivateKey;
// typedef struct Signature Signature;
//
// KeyPair* hashsig_keypair_from_secret_key(const uint8_t* private_key_ptr, size_t private_key_len);
// void hashsig_keypair_free(KeyPair* keypair);
// const PublicKey* hashsig_keypair_get_public_key(const KeyPair* keypair);
// const PrivateKey* hashsig_keypair_get_private_key(const KeyPair* keypair);
// Signature* hashsig_sign(const PrivateKey* private_key, const uint8_t* message_ptr, uint32_t epoch);
// void hashsig_signature_free(Signature* signature);
// size_t hashsig_signature_to_bytes(const Signature* signature, uint8_t* buffer, size_t buffer_len);
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/geanlabs/gean/internal/types"
	"gopkg.in/yaml.v3"
)

type ValidatorKeyPair struct {
	handle *C.KeyPair
	Index  uint64
}

func (kp *ValidatorKeyPair) PublicKeyPtr() *C.PublicKey {
	if kp == nil || kp.handle == nil {
		return nil
	}
	return C.hashsig_keypair_get_public_key(kp.handle)
}

func (kp *ValidatorKeyPair) PublicKey() CPubKey {
	return unsafe.Pointer(kp.PublicKeyPtr())
}

func (kp *ValidatorKeyPair) PrivateKeyPtr() *C.PrivateKey {
	if kp == nil || kp.handle == nil {
		return nil
	}
	return C.hashsig_keypair_get_private_key(kp.handle)
}

func (kp *ValidatorKeyPair) Sign(slot uint32, message [32]byte) ([types.SignatureSize]byte, error) {
	var result [types.SignatureSize]byte
	privateKey := kp.PrivateKeyPtr()
	if privateKey == nil {
		return result, fmt.Errorf("%w: keypair is nil or closed", ErrSigningFailed)
	}

	sigPtr := C.hashsig_sign(
		privateKey,
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
	)
	if sigPtr == nil {
		return result, fmt.Errorf("%w: validator %d slot %d", ErrSigningFailed, kp.Index, slot)
	}
	defer C.hashsig_signature_free(sigPtr)

	buf := make([]byte, SignatureBuffer)
	n := C.hashsig_signature_to_bytes(
		sigPtr,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 || int(n) != types.SignatureSize {
		return result, fmt.Errorf("signature serialization failed: wrote %d bytes, expected %d", n, types.SignatureSize)
	}

	copy(result[:], buf[:n])
	return result, nil
}

func (kp *ValidatorKeyPair) Close() {
	if kp != nil && kp.handle != nil {
		C.hashsig_keypair_free(kp.handle)
		kp.handle = nil
	}
}

type KeyManager struct {
	attestationKeys map[uint64]*ValidatorKeyPair
	proposalKeys    map[uint64]*ValidatorKeyPair
}

func NewKeyManager(attestationKeys, proposalKeys map[uint64]*ValidatorKeyPair) *KeyManager {
	return &KeyManager{
		attestationKeys: attestationKeys,
		proposalKeys:    proposalKeys,
	}
}

func (km *KeyManager) ValidatorIDs() []uint64 {
	if km == nil {
		return nil
	}
	ids := make([]uint64, 0, len(km.attestationKeys))
	for id := range km.attestationKeys {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func (km *KeyManager) GetAttestationKey(validatorID uint64) *ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.attestationKeys[validatorID]
}

func (km *KeyManager) GetProposalKey(validatorID uint64) *ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.proposalKeys[validatorID]
}

func (km *KeyManager) SignAttestation(validatorID uint64, data *types.AttestationData) ([types.SignatureSize]byte, error) {
	if km == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("key manager is nil")
	}
	kp := km.GetAttestationKey(validatorID)
	if kp == nil || kp.handle == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation key for validator %d not found", validatorID)
	}
	if data == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation data is nil")
	}

	msgRoot, err := data.HashTreeRoot()
	if err != nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("hash tree root failed: %w", err)
	}

	slot := uint32(data.Slot)
	if uint64(slot) != data.Slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", data.Slot)
	}

	return kp.Sign(slot, msgRoot)
}

func (km *KeyManager) SignBlock(validatorID uint64, slot uint64, blockRoot [32]byte) ([types.SignatureSize]byte, error) {
	if km == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("key manager is nil")
	}
	kp := km.GetProposalKey(validatorID)
	if kp == nil || kp.handle == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("proposal key for validator %d not found", validatorID)
	}

	s := uint32(slot)
	if uint64(s) != slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", slot)
	}

	return kp.Sign(s, blockRoot)
}

func (km *KeyManager) Close() {
	if km == nil {
		return
	}
	for _, kp := range km.attestationKeys {
		kp.Close()
	}
	for _, kp := range km.proposalKeys {
		kp.Close()
	}
}

// MatchRegistry checks every loaded key against its validator's registry pubkeys, so a swapped,
// shared, or foreign key file stops the node before it signs anything.
func (km *KeyManager) MatchRegistry(validators []*types.Validator) error {
	for _, id := range km.ValidatorIDs() {
		if id >= uint64(len(validators)) {
			return fmt.Errorf("validator %d not in registry of %d validators", id, len(validators))
		}
		if err := matchPubkey(km.attestationKeys[id], validators[id].AttestationPubkey, id, "attestation"); err != nil {
			return err
		}
		if err := matchPubkey(km.proposalKeys[id], validators[id].ProposalPubkey, id, "proposal"); err != nil {
			return err
		}
	}
	return nil
}

func matchPubkey(kp *ValidatorKeyPair, want [types.PubkeySize]byte, id uint64, role string) error {
	got, err := kp.PublicKeyBytes()
	if err != nil {
		return fmt.Errorf("validator %d %s key: %w", id, role, err)
	}
	if got != want {
		return fmt.Errorf("validator %d %s key 0x%x does not match registry pubkey 0x%x", id, role, got, want)
	}
	return nil
}

type annotatedValidator struct {
	Index             uint64 `yaml:"index"`
	PrivkeyFile       string `yaml:"privkey_file"`
	AttestationSkFile string `yaml:"attestation_sk_file"`
	ProposalSkFile    string `yaml:"proposal_sk_file"`
}

// LoadValidatorKeys loads a node's attestation and proposal keys. Entries use either gean keygen's
// attestation_sk_file/proposal_sk_file pair or lean-quickstart's one privkey_file per role, with
// the role named in the file name. Every validator must end up with exactly one key per role.
func LoadValidatorKeys(annotatedPath, keysDir, nodeID string) (*KeyManager, error) {
	data, err := os.ReadFile(annotatedPath)
	if err != nil {
		return nil, fmt.Errorf("read annotated validators: %w", err)
	}

	var allValidators map[string][]annotatedValidator
	if err := yaml.Unmarshal(data, &allValidators); err != nil {
		return nil, fmt.Errorf("parse annotated validators: %w", err)
	}

	validators, ok := allValidators[nodeID]
	if !ok {
		return nil, fmt.Errorf("node ID %q not found in annotated validators", nodeID)
	}

	attestationFiles := make(map[uint64]string)
	proposalFiles := make(map[uint64]string)
	assign := func(files map[uint64]string, role string, index uint64, file string) error {
		if file == "" {
			return fmt.Errorf("%s key file missing for validator %d", role, index)
		}
		if files[index] != "" {
			return fmt.Errorf("duplicate %s key for validator %d", role, index)
		}
		files[index] = file
		return nil
	}

	for _, v := range validators {
		if v.PrivkeyFile != "" {
			if v.AttestationSkFile != "" || v.ProposalSkFile != "" {
				return nil, fmt.Errorf("validator %d: privkey_file cannot be combined with attestation_sk_file or proposal_sk_file", v.Index)
			}
			name := filepath.Base(v.PrivkeyFile)
			attestation := strings.Contains(name, "attester") || strings.Contains(name, "attestation")
			proposal := strings.Contains(name, "proposer") || strings.Contains(name, "proposal")
			if attestation == proposal {
				return nil, fmt.Errorf("validator %d: key file %q must name exactly one role (attester or proposer)", v.Index, v.PrivkeyFile)
			}
			if attestation {
				err = assign(attestationFiles, "attestation", v.Index, v.PrivkeyFile)
			} else {
				err = assign(proposalFiles, "proposal", v.Index, v.PrivkeyFile)
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		if err := assign(attestationFiles, "attestation", v.Index, v.AttestationSkFile); err != nil {
			return nil, err
		}
		if err := assign(proposalFiles, "proposal", v.Index, v.ProposalSkFile); err != nil {
			return nil, err
		}
	}
	for index := range attestationFiles {
		if proposalFiles[index] == "" {
			return nil, fmt.Errorf("proposal key file missing for validator %d", index)
		}
	}
	for index := range proposalFiles {
		if attestationFiles[index] == "" {
			return nil, fmt.Errorf("attestation key file missing for validator %d", index)
		}
	}

	attestationKeys := make(map[uint64]*ValidatorKeyPair, len(attestationFiles))
	proposalKeys := make(map[uint64]*ValidatorKeyPair, len(proposalFiles))
	for index, file := range attestationFiles {
		kp, err := loadKeypair(keysDir, file, index)
		if err != nil {
			return nil, fmt.Errorf("load attestation key for validator %d (%s): %w", index, file, err)
		}
		attestationKeys[index] = kp
	}
	for index, file := range proposalFiles {
		kp, err := loadKeypair(keysDir, file, index)
		if err != nil {
			return nil, fmt.Errorf("load proposal key for validator %d (%s): %w", index, file, err)
		}
		proposalKeys[index] = kp
	}

	return NewKeyManager(attestationKeys, proposalKeys), nil
}

func loadKeypair(keysDir, skFile string, index uint64) (*ValidatorKeyPair, error) {
	skPath := skFile
	if !filepath.IsAbs(skPath) {
		skPath = filepath.Join(keysDir, skFile)
	}

	skBytes, err := os.ReadFile(skPath)
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if len(skBytes) == 0 {
		return nil, fmt.Errorf("secret key is empty")
	}

	handle := C.hashsig_keypair_from_secret_key((*C.uint8_t)(unsafe.Pointer(&skBytes[0])), C.size_t(len(skBytes)))
	if handle == nil {
		return nil, fmt.Errorf("%w: validator %d", ErrKeypairParseFailed, index)
	}

	return &ValidatorKeyPair{handle: handle, Index: index}, nil
}
