package xmss

// #cgo CFLAGS: -I.
// #cgo linux LDFLAGS: -L${SRCDIR}/rust/target/multisig-release -lcgo_glue -lm -ldl -lpthread
// #cgo darwin LDFLAGS: -L${SRCDIR}/rust/target/multisig-release -lcgo_glue -lm -ldl -lpthread -framework CoreFoundation -framework SystemConfiguration -framework Security
//
// #include <stdint.h>
// #include <stdlib.h>
// #include <stdbool.h>
//
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
// void xmss_setup_prover();
// void xmss_setup_verifier();
//
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

	"github.com/geanlabs/gean/internal/types"
)

const (
	MessageLength    = 32
	MaxProofSize     = 1 << 20
	SignatureBuffer  = 4000
	PubkeyBuffer     = 256
	PrivateKeyBuffer = 10 << 20
)

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
	ErrMalformedChildProof = errors.New("malformed child proof")
	ErrMalformedRawInput   = errors.New("malformed raw signature input")
)

var (
	proverOnce   sync.Once
	verifierOnce sync.Once
)

func EnsureProverReady() {
	proverOnce.Do(func() {
		C.xmss_setup_prover()
	})
}

func EnsureVerifierReady() {
	verifierOnce.Do(func() {
		C.xmss_setup_verifier()
	})
}

func VerifySignatureSSZ(pubkey [types.PubkeySize]byte, slot uint32, message [32]byte, signature [types.SignatureSize]byte) (bool, error) {
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

type ChildProof struct {
	Pubkeys   []CPubKey
	ProofData []byte
}

const LogInvRate = 2

func AggregateSignatures(
	pubkeys []CPubKey,
	sigs []CSig,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	return AggregateWithChildren(pubkeys, sigs, nil, message, slot)
}

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
	if err := validateRawInputs(pubkeys, sigs); err != nil {
		return nil, err
	}
	if numRaw == 0 && numChildren < 2 {
		return nil, fmt.Errorf("at least 2 children required when no raw sigs provided")
	}
	if err := validateChildProofs(children); err != nil {
		return nil, err
	}

	EnsureProverReady()

	// Pin Go memory passed to C. Go 1.21+ cgo checks reject passing slices
	// containing Go pointers unless the backing memory is pinned.
	var pinner runtime.Pinner
	defer pinner.Unpin()

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

	var childAllPkPtr **C.PublicKey
	var childNumKeysPtr *C.size_t
	var childProofPtrsPtr **C.uint8_t
	var childProofLensPtr *C.size_t

	if numChildren > 0 {
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

func validateRawInputs(pubkeys []CPubKey, sigs []CSig) error {
	if err := validatePublicKeys(pubkeys); err != nil {
		return err
	}
	for i, sig := range sigs {
		if sig == nil {
			return fmt.Errorf("%w: signature %d is nil", ErrMalformedRawInput, i)
		}
	}
	return nil
}

func validatePublicKeys(pubkeys []CPubKey) error {
	for i, pk := range pubkeys {
		if pk == nil {
			return fmt.Errorf("%w: pubkey %d is nil", ErrMalformedRawInput, i)
		}
	}
	return nil
}

func validateChildProofs(children []ChildProof) error {
	for i, child := range children {
		if len(child.Pubkeys) == 0 {
			return fmt.Errorf("%w: child %d has no pubkeys", ErrMalformedChildProof, i)
		}
		if len(child.ProofData) == 0 {
			return fmt.Errorf("%w: child %d proof data is empty", ErrMalformedChildProof, i)
		}
		for j, pk := range child.Pubkeys {
			if pk == nil {
				return fmt.Errorf("%w: child %d pubkey %d is nil", ErrMalformedChildProof, i, j)
			}
		}
	}
	return nil
}

func VerifyAggregatedSignature(
	proofData []byte,
	pubkeys []CPubKey,
	message [32]byte,
	slot uint32,
) error {
	if len(proofData) == 0 || len(pubkeys) == 0 {
		return ErrEmptyInput
	}
	if err := validatePublicKeys(pubkeys); err != nil {
		return err
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

type CPubKey = unsafe.Pointer

type CSig = unsafe.Pointer

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

func FreePublicKey(pk CPubKey) {
	if pk != nil {
		C.hashsig_public_key_free((*C.PublicKey)(pk))
	}
}

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

func FreeSignature(sig CSig) {
	if sig != nil {
		C.hashsig_signature_free((*C.Signature)(sig))
	}
}

func GenerateKeyPair(seedPhrase string, activationEpoch, numActiveEpochs uint64) (*ValidatorKeyPair, error) {
	cSeed := C.CString(seedPhrase)
	defer C.free(unsafe.Pointer(cSeed))

	kp := C.hashsig_keypair_generate(cSeed, C.size_t(activationEpoch), C.size_t(numActiveEpochs))
	if kp == nil {
		return nil, ErrKeypairParseFailed
	}
	return &ValidatorKeyPair{handle: kp, Index: 0}, nil
}

func (kp *ValidatorKeyPair) PublicKeyBytes() ([types.PubkeySize]byte, error) {
	var result [types.PubkeySize]byte
	publicKey := kp.PublicKeyPtr()
	if publicKey == nil {
		return result, fmt.Errorf("keypair is nil or closed")
	}
	var buf [PubkeyBuffer]byte

	n := C.hashsig_public_key_to_bytes(
		publicKey,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 || int(n) != types.PubkeySize {
		return result, fmt.Errorf("pubkey serialization failed: got %d bytes", n)
	}
	copy(result[:], buf[:n])
	return result, nil
}

func (kp *ValidatorKeyPair) PrivateKeyBytes() ([]byte, error) {
	privateKey := kp.PrivateKeyPtr()
	if privateKey == nil {
		return nil, fmt.Errorf("keypair is nil or closed")
	}
	buf := make([]byte, PrivateKeyBuffer)
	n := C.hashsig_private_key_to_bytes(
		privateKey,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 {
		return nil, fmt.Errorf("private key serialization failed")
	}
	return buf[:n], nil
}
