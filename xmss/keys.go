package xmss

// Key management for XMSS validators.

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
// Valid only while the ValidatorKeyPair is alive — do NOT free it.
func (kp *ValidatorKeyPair) PublicKeyPtr() *C.PublicKey {
	return C.hashsig_keypair_get_public_key(kp.handle)
}

// PrivateKeyPtr returns a borrowed pointer to the embedded private key.
// Valid only while the ValidatorKeyPair is alive.
func (kp *ValidatorKeyPair) PrivateKeyPtr() *C.PrivateKey {
	return C.hashsig_keypair_get_private_key(kp.handle)
}

// Sign signs a 32-byte message at the given slot.
// Returns the SSZ-encoded 3112-byte signature.
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

	// Serialize to fixed-size SSZ bytes.
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

// KeyManager holds all validator keypairs for this node.
type KeyManager struct {
	keys map[uint64]*ValidatorKeyPair // validator_id -> keypair
}

// NewKeyManager creates a KeyManager from loaded keypairs.
func NewKeyManager(keys map[uint64]*ValidatorKeyPair) *KeyManager {
	return &KeyManager{keys: keys}
}

// ValidatorIDs returns all validator indices managed by this node.
func (km *KeyManager) ValidatorIDs() []uint64 {
	ids := make([]uint64, 0, len(km.keys))
	for id := range km.keys {
		ids = append(ids, id)
	}
	return ids
}

// Get returns the keypair for a validator, or nil if not found.
func (km *KeyManager) Get(validatorID uint64) *ValidatorKeyPair {
	return km.keys[validatorID]
}

// SignAttestation signs attestation data for a validator.
// Message = HashTreeRoot(attestationData), slot from the data.
func (km *KeyManager) SignAttestation(validatorID uint64, data *types.AttestationData) ([types.SignatureSize]byte, error) {
	kp, ok := km.keys[validatorID]
	if !ok {
		return [types.SignatureSize]byte{}, fmt.Errorf("validator %d not found in key manager", validatorID)
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

// SignBlock signs a block root for a validator (proposer signature).
func (km *KeyManager) SignBlock(validatorID uint64, slot uint64, blockRoot [32]byte) ([types.SignatureSize]byte, error) {
	kp, ok := km.keys[validatorID]
	if !ok {
		return [types.SignatureSize]byte{}, fmt.Errorf("validator %d not found in key manager", validatorID)
	}

	s := uint32(slot)
	if uint64(s) != slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", slot)
	}

	return kp.Sign(s, blockRoot)
}

// Close frees all keypairs.
func (km *KeyManager) Close() {
	for _, kp := range km.keys {
		kp.Close()
	}
}

// --- Key loading from YAML + files ---

// annotatedValidator represents a validator entry from annotated_validators.yaml.
type annotatedValidator struct {
	Index      uint64 `yaml:"index"`
	PubkeyHex  string `yaml:"pubkey_hex"`
	PrivkeyFile string `yaml:"privkey_file"`
}

// LoadValidatorKeys loads XMSS keypairs from annotated_validators.yaml + key files.
//
// annotatedPath: path to annotated_validators.yaml
// keysDir: directory containing validator_*_sk.ssz files
// nodeID: e.g., "gean_0"
func LoadValidatorKeys(annotatedPath, keysDir, nodeID string) (*KeyManager, error) {
	data, err := os.ReadFile(annotatedPath)
	if err != nil {
		return nil, fmt.Errorf("read annotated validators: %w", err)
	}

	// File is map[node_id][]annotatedValidator.
	var allValidators map[string][]annotatedValidator
	if err := yaml.Unmarshal(data, &allValidators); err != nil {
		return nil, fmt.Errorf("parse annotated validators: %w", err)
	}

	validators, ok := allValidators[nodeID]
	if !ok {
		return nil, fmt.Errorf("node ID %q not found in annotated validators", nodeID)
	}

	keys := make(map[uint64]*ValidatorKeyPair, len(validators))

	for _, v := range validators {
		// Resolve privkey file path (relative to keysDir).
		skPath := v.PrivkeyFile
		if !filepath.IsAbs(skPath) {
			skPath = filepath.Join(keysDir, skPath)
		}

		// Read raw SSZ secret key bytes.
		skBytes, err := os.ReadFile(skPath)
		if err != nil {
			return nil, fmt.Errorf("read secret key for validator %d: %w", v.Index, err)
		}

		// Decode pubkey from hex.
		pkHex := strings.TrimPrefix(strings.TrimSpace(v.PubkeyHex), "0x")
		pkBytes, err := hex.DecodeString(pkHex)
		if err != nil {
			return nil, fmt.Errorf("decode pubkey hex for validator %d: %w", v.Index, err)
		}
		if len(pkBytes) != types.PubkeySize {
			return nil, fmt.Errorf("pubkey for validator %d has %d bytes, expected %d", v.Index, len(pkBytes), types.PubkeySize)
		}

		// Create keypair via FFI.
		handle := C.hashsig_keypair_from_ssz(
			(*C.uint8_t)(unsafe.Pointer(&skBytes[0])), C.size_t(len(skBytes)),
			(*C.uint8_t)(unsafe.Pointer(&pkBytes[0])), C.size_t(len(pkBytes)),
		)
		if handle == nil {
			return nil, fmt.Errorf("%w: validator %d", ErrKeypairParseFailed, v.Index)
		}

		keys[v.Index] = &ValidatorKeyPair{
			handle: handle,
			Index:  v.Index,
		}
	}

	return NewKeyManager(keys), nil
}
