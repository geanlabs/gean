package xmss

// CGo bindings to gean's Rust glue libraries (hashsig-glue + multisig-glue).
//
// Build the FFI libraries first:
//   make ffi
//
// Static libraries are compiled to crypto/rust/target/release/.

// #cgo CFLAGS: -I.
// #cgo linux LDFLAGS: -L${SRCDIR}/rust/target/release -lhashsig_glue -lmultisig_glue -lm -ldl -lpthread
// #cgo darwin LDFLAGS: -L${SRCDIR}/rust/target/release -lhashsig_glue -lmultisig_glue -lm -ldl -lpthread -framework CoreFoundation -framework SystemConfiguration -framework Security
//
// #include <stdint.h>
// #include <stdlib.h>
// #include <stdbool.h>
//
// // Opaque types from hashsig-glue
// typedef struct KeyPair KeyPair;
// typedef struct PublicKey PublicKey;
// typedef struct PrivateKey PrivateKey;
// typedef struct Signature Signature;
//
// // Opaque type from multisig-glue
// typedef struct Devnet2XmssAggregateSignature Devnet2XmssAggregateSignature;
//
// // --- hashsig-glue FFI (rs) ---
//
// KeyPair* hashsig_keypair_from_ssz(
//     const uint8_t* private_key_ptr, size_t private_key_len,
//     const uint8_t* public_key_ptr, size_t public_key_len);
// void hashsig_keypair_free(KeyPair* keypair);
// const PublicKey* hashsig_keypair_get_public_key(const KeyPair* keypair);
// const PrivateKey* hashsig_keypair_get_private_key(const KeyPair* keypair);
//
// PublicKey* hashsig_public_key_from_ssz(const uint8_t* public_key_ptr, size_t public_key_len);
// void hashsig_public_key_free(PublicKey* public_key);
//
// Signature* hashsig_sign(const PrivateKey* private_key, const uint8_t* message_ptr, uint32_t epoch);
// void hashsig_signature_free(Signature* signature);
// size_t hashsig_signature_to_bytes(const Signature* signature, uint8_t* buffer, size_t buffer_len);
// Signature* hashsig_signature_from_ssz(const uint8_t* signature_ptr, size_t signature_len);
//
// int hashsig_verify(const PublicKey* public_key, const uint8_t* message_ptr,
//     uint32_t epoch, const Signature* signature);
// int hashsig_verify_ssz(const uint8_t* pubkey_bytes, size_t pubkey_len,
//     const uint8_t* message, uint32_t epoch,
//     const uint8_t* signature_bytes, size_t signature_len);
//
// size_t hashsig_public_key_to_bytes(const PublicKey* public_key, uint8_t* buffer, size_t buffer_len);
// size_t hashsig_private_key_to_bytes(const PrivateKey* private_key, uint8_t* buffer, size_t buffer_len);
// size_t hashsig_message_length();
//
// KeyPair* hashsig_keypair_generate(const char* seed_phrase,
//     size_t activation_epoch, size_t num_active_epochs);
//
// // --- multisig-glue FFI (rs) ---
//
// void xmss_setup_prover();
// void xmss_setup_verifier();
//
// const Devnet2XmssAggregateSignature* xmss_aggregate(
//     const PublicKey* const* public_keys, size_t num_keys,
//     const Signature* const* signatures, size_t num_sigs,
//     const uint8_t* message_hash_ptr, uint32_t epoch);
//
// bool xmss_verify_aggregated(
//     const PublicKey* const* public_keys, size_t num_keys,
//     const uint8_t* message_hash_ptr,
//     const Devnet2XmssAggregateSignature* agg_sig, uint32_t epoch);
//
// void xmss_free_aggregate_signature(Devnet2XmssAggregateSignature* agg_sig);
// size_t xmss_aggregate_signature_to_bytes(
//     const Devnet2XmssAggregateSignature* agg_sig,
//     uint8_t* buffer, size_t buffer_len);
// Devnet2XmssAggregateSignature* xmss_aggregate_signature_from_bytes(
//     const uint8_t* bytes, size_t bytes_len);
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/geanlabs/gean/types"
)

// Size constants.
const (
	MessageLength   = 32
	MaxProofSize    = 1 << 20 // 1 MiB (ByteListMiB)
	SignatureBuffer = 4000    // buffer for SSZ signature serialization
	PubkeyBuffer    = 256     // buffer for SSZ pubkey serialization
)

// Errors.
var (
	ErrEmptyInput          = errors.New("empty input")
	ErrCountMismatch       = errors.New("public key count does not match signature count")
	ErrAggregationFailed   = errors.New("signature aggregation failed")
	ErrSerializationFailed = errors.New("proof serialization failed")
	ErrProofTooBig         = errors.New("aggregated proof exceeds 1 MiB")
	ErrVerificationFailed  = errors.New("aggregated signature verification failed")
	ErrDeserializeFailed   = errors.New("proof deserialization failed")
	ErrSigningFailed       = errors.New("signing failed")
	ErrInvalidSignature    = errors.New("signature verification returned invalid")
	ErrSignatureError      = errors.New("signature verification error (malformed data)")
	ErrPubkeyParseFailed   = errors.New("public key parsing failed")
	ErrKeypairParseFailed  = errors.New("keypair parsing failed")
)

// Lazy initialization guards
var (
	proverOnce   sync.Once
	verifierOnce sync.Once
)

// EnsureProverReady initializes the aggregation prover (expensive, runs once).
// Matches multisig-glue xmss_setup_prover with PROVER_INIT Once guard.
func EnsureProverReady() {
	proverOnce.Do(func() {
		C.xmss_setup_prover()
	})
}

// EnsureVerifierReady initializes the aggregation verifier (runs once).
// Matches multisig-glue xmss_setup_verifier with VERIFIER_INIT Once guard.
func EnsureVerifierReady() {
	verifierOnce.Do(func() {
		C.xmss_setup_verifier()
	})
}

// VerifySignatureSSZ verifies an individual XMSS signature from raw bytes.
// No aggregation VM setup needed — single sig verification is standalone.
// Returns (valid, error). Error means malformed input, not invalid signature.
func VerifySignatureSSZ(pubkey [types.PubkeySize]byte, slot uint32, message [32]byte, signature [types.SignatureSize]byte) (bool, error) {
	// No EnsureVerifierReady() — that's for aggregation only.
	// Single sig verify is stateless (matches old gean pattern).

	result := C.hashsig_verify_ssz(
		(*C.uint8_t)(unsafe.Pointer(&pubkey[0])), C.size_t(types.PubkeySize),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
		(*C.uint8_t)(unsafe.Pointer(&signature[0])), C.size_t(types.SignatureSize),
	)

	switch result {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, ErrSignatureError
	}
}

// AggregateSignatures aggregates multiple XMSS signatures into a single ZK proof.
// Takes arrays of opaque PublicKey/Signature pointers from resolved gossip signatures.
// Returns SSZ-encoded proof bytes (max 1 MiB).
func AggregateSignatures(
	pubkeys []CPubKey,
	sigs []CSig,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	if len(pubkeys) == 0 {
		return nil, ErrEmptyInput
	}
	if len(pubkeys) != len(sigs) {
		return nil, fmt.Errorf("%w: %d pubkeys, %d sigs", ErrCountMismatch, len(pubkeys), len(sigs))
	}

	EnsureProverReady()

	// Convert []CPubKey ([]unsafe.Pointer) to []*C.PublicKey for FFI.
	cPubkeys := make([]*C.PublicKey, len(pubkeys))
	for i, pk := range pubkeys {
		cPubkeys[i] = (*C.PublicKey)(pk)
	}
	cSigs := make([]*C.Signature, len(sigs))
	for i, s := range sigs {
		cSigs[i] = (*C.Signature)(s)
	}

	aggSig := C.xmss_aggregate(
		(**C.PublicKey)(unsafe.Pointer(&cPubkeys[0])),
		C.size_t(len(cPubkeys)),
		(**C.Signature)(unsafe.Pointer(&cSigs[0])),
		C.size_t(len(cSigs)),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
	)
	if aggSig == nil {
		return nil, ErrAggregationFailed
	}
	defer C.xmss_free_aggregate_signature((*C.Devnet2XmssAggregateSignature)(unsafe.Pointer(aggSig)))

	// Serialize to SSZ bytes using pooled buffer.
	bufPtr := getProofBuf()
	buf := *bufPtr
	n := C.xmss_aggregate_signature_to_bytes(
		aggSig,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 {
		putProofBuf(bufPtr)
		return nil, ErrSerializationFailed
	}
	if int(n) > MaxProofSize {
		putProofBuf(bufPtr)
		return nil, ErrProofTooBig
	}

	// Copy used bytes to a right-sized slice, return pooled buffer.
	result := make([]byte, int(n))
	copy(result, buf[:n])
	putProofBuf(bufPtr)
	return result, nil
}

// VerifyAggregatedSignature verifies an aggregated XMSS proof.
// Takes SSZ proof bytes + array of pubkey pointers for participating validators.
func VerifyAggregatedSignature(
	proofData []byte,
	pubkeys []CPubKey,
	message [32]byte,
	slot uint32,
) error {
	if len(proofData) == 0 || len(pubkeys) == 0 {
		return ErrEmptyInput
	}

	EnsureVerifierReady()

	// Deserialize proof from SSZ bytes.
	aggSig := C.xmss_aggregate_signature_from_bytes(
		(*C.uint8_t)(unsafe.Pointer(&proofData[0])),
		C.size_t(len(proofData)),
	)
	if aggSig == nil {
		return ErrDeserializeFailed
	}
	defer C.xmss_free_aggregate_signature(aggSig)

	cPubkeys := make([]*C.PublicKey, len(pubkeys))
	for i, pk := range pubkeys {
		cPubkeys[i] = (*C.PublicKey)(pk)
	}

	valid := C.xmss_verify_aggregated(
		(**C.PublicKey)(unsafe.Pointer(&cPubkeys[0])),
		C.size_t(len(cPubkeys)),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		(*C.Devnet2XmssAggregateSignature)(unsafe.Pointer(aggSig)),
		C.uint32_t(slot),
	)
	if !valid {
		return ErrVerificationFailed
	}

	return nil
}

// CPubKey is an opaque handle to a C PublicKey, exported as unsafe.Pointer
// so other packages can hold and pass it without importing C types.
type CPubKey = unsafe.Pointer

// CSig is an opaque handle to a C Signature.
type CSig = unsafe.Pointer

// ParsePublicKey creates an opaque PublicKey handle from SSZ-encoded bytes.
// Caller must call FreePublicKey when done.
func ParsePublicKey(pubkeyBytes [types.PubkeySize]byte) (CPubKey, error) {
	pk := C.hashsig_public_key_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&pubkeyBytes[0])),
		C.size_t(types.PubkeySize),
	)
	if pk == nil {
		return nil, ErrPubkeyParseFailed
	}
	return unsafe.Pointer(pk), nil
}

// FreePublicKey frees a PublicKey handle created by ParsePublicKey.
func FreePublicKey(pk CPubKey) {
	if pk != nil {
		C.hashsig_public_key_free((*C.PublicKey)(pk))
	}
}

// ParseSignature creates an opaque Signature handle from SSZ-encoded bytes.
// Caller must call FreeSignature when done.
func ParseSignature(sigBytes []byte) (CSig, error) {
	if len(sigBytes) == 0 {
		return nil, ErrInvalidSignature
	}
	sig := C.hashsig_signature_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&sigBytes[0])),
		C.size_t(len(sigBytes)),
	)
	if sig == nil {
		return nil, ErrInvalidSignature
	}
	return unsafe.Pointer(sig), nil
}

// FreeSignature frees a Signature handle created by ParseSignature.
func FreeSignature(sig CSig) {
	if sig != nil {
		C.hashsig_signature_free((*C.Signature)(sig))
	}
}

// GenerateKeyPair generates a new XMSS keypair from a seed phrase.
// Used for testing. The returned ValidatorKeyPair must be closed when done.
// Matches hashsig-glue hashsig_keypair_generate.
func GenerateKeyPair(seedPhrase string, activationEpoch, numActiveEpochs uint64) (*ValidatorKeyPair, error) {
	cSeed := C.CString(seedPhrase)
	defer C.free(unsafe.Pointer(cSeed))

	kp := C.hashsig_keypair_generate(cSeed, C.size_t(activationEpoch), C.size_t(numActiveEpochs))
	if kp == nil {
		return nil, ErrKeypairParseFailed
	}
	return &ValidatorKeyPair{handle: kp, Index: 0}, nil
}

// PublicKeyBytes returns the SSZ-encoded public key bytes from a ValidatorKeyPair.
func (kp *ValidatorKeyPair) PublicKeyBytes() ([types.PubkeySize]byte, error) {
	var result [types.PubkeySize]byte
	var buf [PubkeyBuffer]byte

	n := C.hashsig_public_key_to_bytes(
		kp.PublicKeyPtr(),
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 || int(n) != types.PubkeySize {
		return result, fmt.Errorf("pubkey serialization failed: got %d bytes", n)
	}
	copy(result[:], buf[:n])
	return result, nil
}
