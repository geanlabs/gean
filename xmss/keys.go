package xmss

// Key management for XMSS validators.
// Devnet-4: dual keys — separate attestation and proposal keypairs per validator.
// Spec: lean_spec/subspecs/containers/validator.py

// #include <stdint.h>
// #include <stdlib.h>
// typedef struct KeyPair KeyPair;
// typedef struct PublicKey PublicKey;
// typedef struct PrivateKey PrivateKey;
// typedef struct Signature Signature;
//
// KeyPair* hashsig_keypair_from_ssz(
//     const uint8_t* private_key_ptr, size_t private_key_len,
//     const uint8_t* public_key_ptr, size_t public_key_len);
// void hashsig_keypair_free(KeyPair* keypair);
// const PublicKey* hashsig_keypair_get_public_key(const KeyPair* keypair);
// const PrivateKey* hashsig_keypair_get_private_key(const KeyPair* keypair);
// Signature* hashsig_sign(const PrivateKey* private_key, const uint8_t* message_ptr, uint32_t epoch);
// void hashsig_signature_free(Signature* signature);
// size_t hashsig_signature_to_bytes(const Signature* signature, uint8_t* buffer, size_t buffer_len);
import "C"

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/geanlabs/gean/types"
	"gopkg.in/yaml.v3"
)

// ValidatorKeyPair holds a loaded XMSS keypair for a validator.
// The opaque C pointer is owned by this struct and freed on Close.
type ValidatorKeyPair struct {
	handle *C.KeyPair
	Index  uint64
}

// PublicKeyPtr returns a borrowed pointer to the embedded public key.
func (kp *ValidatorKeyPair) PublicKeyPtr() *C.PublicKey {
	return C.hashsig_keypair_get_public_key(kp.handle)
}

// PrivateKeyPtr returns a borrowed pointer to the embedded private key.
func (kp *ValidatorKeyPair) PrivateKeyPtr() *C.PrivateKey {
	return C.hashsig_keypair_get_private_key(kp.handle)
}

// Sign signs a 32-byte message at the given slot.
func (kp *ValidatorKeyPair) Sign(slot uint32, message [32]byte) ([types.SignatureSize]byte, error) {
	var result [types.SignatureSize]byte

	sigPtr := C.hashsig_sign(
		kp.PrivateKeyPtr(),
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

// Close frees the underlying C keypair.
func (kp *ValidatorKeyPair) Close() {
	if kp.handle != nil {
		C.hashsig_keypair_free(kp.handle)
		kp.handle = nil
	}
}

// KeyManager holds dual keypairs for each validator on this node.
// Attestation and proposal keys are independent, allowing both to sign
// in the same slot without violating XMSS one-time signature constraints.
type KeyManager struct {
	attestationKeys map[uint64]*ValidatorKeyPair
	proposalKeys    map[uint64]*ValidatorKeyPair
}

// NewKeyManager creates a KeyManager from loaded dual keypairs.
func NewKeyManager(attestationKeys, proposalKeys map[uint64]*ValidatorKeyPair) *KeyManager {
	return &KeyManager{
		attestationKeys: attestationKeys,
		proposalKeys:    proposalKeys,
	}
}

// ValidatorIDs returns all validator indices managed by this node.
func (km *KeyManager) ValidatorIDs() []uint64 {
	ids := make([]uint64, 0, len(km.attestationKeys))
	for id := range km.attestationKeys {
		ids = append(ids, id)
	}
	return ids
}

// GetAttestationKey returns the attestation keypair for a validator.
func (km *KeyManager) GetAttestationKey(validatorID uint64) *ValidatorKeyPair {
	return km.attestationKeys[validatorID]
}

// GetProposalKey returns the proposal keypair for a validator.
func (km *KeyManager) GetProposalKey(validatorID uint64) *ValidatorKeyPair {
	return km.proposalKeys[validatorID]
}

// SignAttestation signs attestation data using the attestation key.
func (km *KeyManager) SignAttestation(validatorID uint64, data *types.AttestationData) ([types.SignatureSize]byte, error) {
	kp, ok := km.attestationKeys[validatorID]
	if !ok {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation key for validator %d not found", validatorID)
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

// SignBlock signs a block root using the proposal key.
func (km *KeyManager) SignBlock(validatorID uint64, slot uint64, blockRoot [32]byte) ([types.SignatureSize]byte, error) {
	kp, ok := km.proposalKeys[validatorID]
	if !ok {
		return [types.SignatureSize]byte{}, fmt.Errorf("proposal key for validator %d not found", validatorID)
	}

	s := uint32(slot)
	if uint64(s) != slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", slot)
	}

	return kp.Sign(s, blockRoot)
}

// Close frees all keypairs.
func (km *KeyManager) Close() {
	for _, kp := range km.attestationKeys {
		kp.Close()
	}
	for _, kp := range km.proposalKeys {
		kp.Close()
	}
}

// --- Key loading from YAML + files ---

// annotatedValidator represents a validator entry from annotated_validators.yaml.
// Supports both formats:
//   - lean-quickstart: two entries per validator with pubkey_hex + privkey_file
//     (attester vs proposer inferred from filename containing "attester" or "proposer")
//   - gean keygen: one entry per validator with attestation/proposal specific fields
type annotatedValidator struct {
	Index uint64 `yaml:"index"`
	// lean-quickstart format (shared by zeam, ethlambda, etc.)
	PubkeyHex   string `yaml:"pubkey_hex"`
	PrivkeyFile string `yaml:"privkey_file"`
	// gean keygen format (dual keys in single entry)
	AttestationPubkey string `yaml:"attestation_pubkey_hex"`
	ProposalPubkey    string `yaml:"proposal_pubkey_hex"`
	AttestationSkFile string `yaml:"attestation_sk_file"`
	ProposalSkFile    string `yaml:"proposal_sk_file"`
}

// LoadValidatorKeys loads dual XMSS keypairs from annotated_validators.yaml + key files.
// Supports lean-quickstart format (two entries per validator: attester + proposer)
// and gean keygen format (one entry per validator with both keys).
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

	attestationKeys := make(map[uint64]*ValidatorKeyPair)
	proposalKeys := make(map[uint64]*ValidatorKeyPair)

	for _, v := range validators {
		if v.PrivkeyFile != "" {
			// lean-quickstart format: pubkey_hex + privkey_file per entry.
			// Attester vs proposer determined by filename.
			kp, err := loadKeypair(keysDir, v.PrivkeyFile, v.PubkeyHex, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load key for validator %d (%s): %w", v.Index, v.PrivkeyFile, err)
			}
			if strings.Contains(v.PrivkeyFile, "attester") || strings.Contains(v.PrivkeyFile, "attestation") {
				attestationKeys[v.Index] = kp
			} else if strings.Contains(v.PrivkeyFile, "proposer") || strings.Contains(v.PrivkeyFile, "proposal") {
				proposalKeys[v.Index] = kp
			} else {
				// Unknown type — use as both (backward compat with single-key format).
				if attestationKeys[v.Index] == nil {
					attestationKeys[v.Index] = kp
				}
				if proposalKeys[v.Index] == nil {
					proposalKeys[v.Index] = kp
				}
			}
		} else if v.AttestationSkFile != "" {
			// gean keygen format: one entry with both keys.
			attKp, err := loadKeypair(keysDir, v.AttestationSkFile, v.AttestationPubkey, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load attestation key for validator %d: %w", v.Index, err)
			}
			attestationKeys[v.Index] = attKp

			propKp, err := loadKeypair(keysDir, v.ProposalSkFile, v.ProposalPubkey, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load proposal key for validator %d: %w", v.Index, err)
			}
			proposalKeys[v.Index] = propKp
		}
	}

	return NewKeyManager(attestationKeys, proposalKeys), nil
}

// loadKeypair loads a single XMSS keypair from an SK file and pubkey hex.
func loadKeypair(keysDir, skFile, pubkeyHex string, index uint64) (*ValidatorKeyPair, error) {
	skPath := skFile
	if !filepath.IsAbs(skPath) {
		skPath = filepath.Join(keysDir, skFile)
	}

	skBytes, err := os.ReadFile(skPath)
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}

	pkHex := strings.TrimPrefix(strings.TrimSpace(pubkeyHex), "0x")
	pkBytes, err := hex.DecodeString(pkHex)
	if err != nil {
		return nil, fmt.Errorf("decode pubkey hex: %w", err)
	}
	if len(pkBytes) != types.PubkeySize {
		return nil, fmt.Errorf("pubkey has %d bytes, expected %d", len(pkBytes), types.PubkeySize)
	}

	handle := C.hashsig_keypair_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&skBytes[0])), C.size_t(len(skBytes)),
		(*C.uint8_t)(unsafe.Pointer(&pkBytes[0])), C.size_t(len(pkBytes)),
	)
	if handle == nil {
		return nil, fmt.Errorf("%w: validator %d", ErrKeypairParseFailed, index)
	}

	return &ValidatorKeyPair{handle: handle, Index: index}, nil
}
