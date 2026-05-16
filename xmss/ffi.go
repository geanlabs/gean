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
// // Recursive aggregation: raw XMSS sigs + children proofs.
// typedef struct AggregatedXMSS AggregatedXMSS;
//
// const AggregatedXMSS* xmss_aggregate(
//     const PublicKey* const* raw_pub_keys,
//     const Signature* const* raw_signatures,
//     size_t num_raw,
//     size_t num_children,
//     const PublicKey* const* child_all_pub_keys,
//     const size_t* child_num_keys,
//     const uint8_t* const* child_proof_ptrs,
//     const size_t* child_proof_lens,
//     const uint8_t* message_hash_ptr,
//     uint32_t slot,
//     size_t log_inv_rate);
//
// bool xmss_verify_aggregated(
//     const PublicKey* const* public_keys, size_t num_keys,
//     const uint8_t* message_hash_ptr,
//     const uint8_t* agg_sig_bytes, size_t agg_sig_len,
//     uint32_t epoch);
//
// void xmss_free_aggregate_signature(AggregatedXMSS* agg_sig);
// size_t xmss_aggregate_signature_to_bytes(
//     const AggregatedXMSS* agg_sig,
//     uint8_t* buffer, size_t buffer_len);
// AggregatedXMSS* xmss_aggregate_signature_from_bytes(
//     const uint8_t* bytes, size_t bytes_len);
import "C"

import (
	"errors"
	"fmt"
	"runtime"
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

// ChildProof represents a pre-aggregated proof with its participants' public keys.
// Used as input to recursive aggregation.
type ChildProof struct {
	Pubkeys   []CPubKey // Parsed public keys of participants
	ProofData []byte    // SSZ-encoded proof bytes
}

// LogInvRate controls proof compression depth.
const LogInvRate = 2 // Production value per spec PROD_CONFIG

// AggregateSignatures aggregates raw XMSS signatures into a single ZK proof.
// Backward-compatible wrapper: no children, just raw sigs.
func AggregateSignatures(
	pubkeys []CPubKey,
	sigs []CSig,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	return AggregateWithChildren(pubkeys, sigs, nil, message, slot)
}

// AggregateWithChildren aggregates raw XMSS signatures + children proofs into
// a single recursive ZK proof. Matches spec AggregatedSignatureProof.aggregate().
// Spec: lean_spec/subspecs/containers/attestation.py AggregatedSignatureProof.aggregate
func AggregateWithChildren(
	pubkeys []CPubKey,
	sigs []CSig,
	children []ChildProof,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	numRaw := len(pubkeys)
	numChildren := len(children)

	if numRaw == 0 && numChildren == 0 {
		return nil, ErrEmptyInput
	}
	if numRaw > 0 && len(sigs) != numRaw {
		return nil, fmt.Errorf("%w: %d pubkeys, %d sigs", ErrCountMismatch, numRaw, len(sigs))
	}
	if numRaw == 0 && numChildren < 2 {
		return nil, fmt.Errorf("at least 2 children required when no raw sigs provided")
	}

	EnsureProverReady()

	// Pin Go memory passed to C. Go 1.21+ cgo checks reject passing slices
	// containing Go pointers unless the backing memory is pinned.
	var pinner runtime.Pinner
	defer pinner.Unpin()

	// Convert raw pubkeys/sigs to C arrays.
	var rawPkPtr **C.PublicKey
	var rawSigPtr **C.Signature
	if numRaw > 0 {
		cPubkeys := make([]*C.PublicKey, numRaw)
		for i, pk := range pubkeys {
			cPubkeys[i] = (*C.PublicKey)(pk)
		}
		cSigs := make([]*C.Signature, numRaw)
		for i, s := range sigs {
			cSigs[i] = (*C.Signature)(s)
		}
		pinner.Pin(&cPubkeys[0])
		pinner.Pin(&cSigs[0])
		rawPkPtr = (**C.PublicKey)(unsafe.Pointer(&cPubkeys[0]))
		rawSigPtr = (**C.Signature)(unsafe.Pointer(&cSigs[0]))
	}

	// Build children arrays for FFI.
	var childAllPkPtr **C.PublicKey
	var childNumKeysPtr *C.size_t
	var childProofPtrsPtr **C.uint8_t
	var childProofLensPtr *C.size_t

	if numChildren > 0 {
		// Flatten all child pubkeys into one array.
		var allChildPks []*C.PublicKey
		childNumKeys := make([]C.size_t, numChildren)
		childProofPtrs := make([]*C.uint8_t, numChildren)
		childProofLens := make([]C.size_t, numChildren)

		for i, child := range children {
			childNumKeys[i] = C.size_t(len(child.Pubkeys))
			for _, pk := range child.Pubkeys {
				allChildPks = append(allChildPks, (*C.PublicKey)(pk))
			}
			pinner.Pin(&child.ProofData[0])
			childProofPtrs[i] = (*C.uint8_t)(unsafe.Pointer(&child.ProofData[0]))
			childProofLens[i] = C.size_t(len(child.ProofData))
		}

		if len(allChildPks) > 0 {
			pinner.Pin(&allChildPks[0])
			childAllPkPtr = (**C.PublicKey)(unsafe.Pointer(&allChildPks[0]))
		}
		pinner.Pin(&childProofPtrs[0])
		childNumKeysPtr = (*C.size_t)(unsafe.Pointer(&childNumKeys[0]))
		childProofPtrsPtr = (**C.uint8_t)(unsafe.Pointer(&childProofPtrs[0]))
		childProofLensPtr = (*C.size_t)(unsafe.Pointer(&childProofLens[0]))
	}

	aggSig := C.xmss_aggregate(
		rawPkPtr,
		rawSigPtr,
		C.size_t(numRaw),
		C.size_t(numChildren),
		childAllPkPtr,
		childNumKeysPtr,
		childProofPtrsPtr,
		childProofLensPtr,
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
		C.size_t(LogInvRate),
	)
	if aggSig == nil {
		return nil, ErrAggregationFailed
	}
	defer C.xmss_free_aggregate_signature((*C.AggregatedXMSS)(unsafe.Pointer(aggSig)))

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

	cPubkeys := make([]*C.PublicKey, len(pubkeys))
	for i, pk := range pubkeys {
		cPubkeys[i] = (*C.PublicKey)(pk)
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&cPubkeys[0])

	valid := C.xmss_verify_aggregated(
		(**C.PublicKey)(unsafe.Pointer(&cPubkeys[0])),
		C.size_t(len(cPubkeys)),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		(*C.uint8_t)(unsafe.Pointer(&proofData[0])),
		C.size_t(len(proofData)),
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
