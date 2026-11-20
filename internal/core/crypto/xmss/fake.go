//go:build fakecrypto

package xmss

// This file replaces the XMSS FFI under the fakecrypto build tag, as
// Lighthouse's fake_crypto replaces BLS: a key pair is derived from its
// secret bytes, signatures and proofs are placeholders, and every
// verification succeeds. Malformed input is still rejected as the real
// library rejects it. It needs no Rust library and costs no CPU, for
// multi-node tests. It accepts any signature, so it never runs a real node.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"unsafe"
)

type ValidatorKeyPair struct {
	secret []byte
	public [PubkeySize]byte
	Index  uint64
}

func KeyPairFromSecret(skBytes []byte, index uint64) (*ValidatorKeyPair, error) {
	return &ValidatorKeyPair{
		secret: append([]byte(nil), skBytes...),
		public: sha256.Sum256(skBytes),
		Index:  index,
	}, nil
}

func GenerateKeyPair(seedPhrase string, activationEpoch, numActiveEpochs uint64) (*ValidatorKeyPair, error) {
	return KeyPairFromSecret([]byte("fake xmss secret key: "+seedPhrase), 0)
}

// loaded reports whether kp holds a key, as the native handle does in the real
// build: a zero-value or closed key pair cannot sign.
func (kp *ValidatorKeyPair) loaded() bool {
	return kp != nil && kp.secret != nil
}

func (kp *ValidatorKeyPair) PublicKey() CPubKey {
	if !kp.loaded() {
		return nil
	}
	return unsafe.Pointer(&kp.public)
}

func (kp *ValidatorKeyPair) PublicKeyBytes() ([PubkeySize]byte, error) {
	if !kp.loaded() {
		return [PubkeySize]byte{}, fmt.Errorf("keypair is nil or closed")
	}
	return kp.public, nil
}

func (kp *ValidatorKeyPair) PrivateKeyBytes() ([]byte, error) {
	if !kp.loaded() {
		return nil, fmt.Errorf("keypair is nil or closed")
	}
	return append([]byte(nil), kp.secret...), nil
}

// Sign returns a placeholder signature that depends on the key, slot and
// message, so distinct signatures stay distinct.
func (kp *ValidatorKeyPair) Sign(slot uint32, message [32]byte) ([SignatureSize]byte, error) {
	var sig [SignatureSize]byte
	if !kp.loaded() {
		return sig, fmt.Errorf("%w: keypair is nil or closed", ErrSigningFailed)
	}
	var slotBytes [4]byte
	binary.LittleEndian.PutUint32(slotBytes[:], slot)
	digest := sha256.Sum256(append(append(kp.public[:], message[:]...), slotBytes[:]...))
	for i := 0; i < SignatureSize; i += len(digest) {
		copy(sig[i:], digest[:])
	}
	return sig, nil
}

func (kp *ValidatorKeyPair) Close() {
	if kp != nil {
		kp.secret = nil
	}
}

func SetProverArena(enabled bool) {}

func EnsureProverReady() error { return nil }

func EnsureVerifierReady() error { return nil }

func ParsePublicKey(pubkeyBytes [PubkeySize]byte) (CPubKey, error) {
	pk := pubkeyBytes
	return unsafe.Pointer(&pk), nil
}

func FreePublicKey(pk CPubKey) {}

func ParseSignature(sigBytes []byte) (CSig, error) {
	if len(sigBytes) == 0 {
		return nil, ErrInvalidSignature
	}
	sig := append([]byte(nil), sigBytes...)
	return unsafe.Pointer(&sig[0]), nil
}

func FreeSignature(sig CSig) {}

func VerifySignatureSSZ(pubkey [PubkeySize]byte, slot uint32, message [32]byte, signature [SignatureSize]byte) (bool, error) {
	return true, nil
}

// fakeProof stands in for a proof of message at slot. Callers only need it
// non-empty.
func fakeProof(message [MessageLength]byte, slot uint32) []byte {
	proof := make([]byte, MessageLength+4)
	copy(proof, message[:])
	binary.LittleEndian.PutUint32(proof[MessageLength:], slot)
	return proof
}

func AggregateWithChildren(pubkeys []CPubKey, sigs []CSig, children []ChildProof, message [32]byte, slot uint32) ([]byte, error) {
	numRaw, numChildren := len(pubkeys), len(children)
	if numRaw == 0 && numChildren == 0 {
		return nil, ErrEmptyInput
	}
	if numRaw > 0 && len(sigs) != numRaw {
		return nil, fmt.Errorf("%w: %d pubkeys, %d sigs", ErrCountMismatch, numRaw, len(sigs))
	}
	if err := validateRawInputs(pubkeys, sigs); err != nil {
		return nil, err
	}
	if numRaw == 0 && numChildren < 2 {
		return nil, fmt.Errorf("at least 2 children required when no raw sigs provided")
	}
	if err := validateChildProofs(children); err != nil {
		return nil, err
	}
	return fakeProof(message, slot), nil
}

func VerifyAggregatedSignature(proofData []byte, pubkeys []CPubKey, message [32]byte, slot uint32) error {
	if len(proofData) == 0 || len(pubkeys) == 0 {
		return ErrEmptyInput
	}
	if err := validatePublicKeys(pubkeys); err != nil {
		return err
	}
	if len(proofData) > MaxProofSize {
		return ErrProofTooBig
	}
	return nil
}

func MergeType1Proofs(inputs []Type1Input, raw []RawSignature) ([]byte, error) {
	if len(inputs) == 0 && len(raw) == 0 {
		return nil, ErrEmptyInput
	}
	keys := 0
	for _, input := range inputs {
		if len(input.Proof) == 0 || len(input.Proof) > MaxProofSize {
			return nil, ErrMalformedChildProof
		}
		if err := validatePublicKeys(input.Pubkeys); err != nil {
			return nil, err
		}
		keys += len(input.Pubkeys)
	}
	if len(inputs) > 0 && keys == 0 {
		return nil, ErrEmptyInput
	}
	for i, r := range raw {
		if r.Pubkey == nil || r.Signature == nil {
			return nil, fmt.Errorf("%w: raw signature %d", ErrMalformedRawInput, i)
		}
	}
	return fakeProof([MessageLength]byte{}, 0), nil
}

func SplitType2Proof(proof []byte, pubkeys [][]CPubKey, bindings []MessageBinding, target [MessageLength]byte) ([]byte, error) {
	if len(proof) == 0 || len(pubkeys) == 0 {
		return nil, ErrEmptyInput
	}
	if len(pubkeys) != len(bindings) {
		return nil, ErrCountMismatch
	}
	if len(proof) > MaxProofSize {
		return nil, ErrProofTooBig
	}
	for _, b := range bindings {
		if b.Message == target {
			return fakeProof(target, b.Slot), nil
		}
	}
	return nil, fmt.Errorf("%w: no binding for the target message", ErrMalformedChildProof)
}

func VerifyType2Proof(proof []byte, pubkeys [][]CPubKey, bindings []MessageBinding) error {
	if len(proof) == 0 || len(pubkeys) == 0 || len(pubkeys) != len(bindings) {
		return ErrCountMismatch
	}
	if len(proof) > MaxProofSize {
		return ErrProofTooBig
	}
	return nil
}
