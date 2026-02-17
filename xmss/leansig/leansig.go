// Package leansig provides Go bindings for the leansig XMSS post-quantum
// signature scheme via CGo. It wraps the Rust leansig-ffi library which
// targets the devnet-1 instantiation (SIGTargetSumLifetime18W1NoOff).
//
// The library must be built before using this package:
//
//	cd xmss/leansig-ffi && cargo build --release
package leansig

/*
#cgo CFLAGS: -I${SRCDIR}/../leansig-ffi/include
#cgo LDFLAGS: ${SRCDIR}/../leansig-ffi/target/release/deps/libleansig_ffi.a -lm -ldl -lpthread
#include "leansig_ffi.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// MessageLength is the fixed size of messages that can be signed (32 bytes).
const MessageLength = 32

// Result codes matching the LeansigResult C enum.
const (
	ResultOK                    = C.LEANSIG_OK
	ResultNullPointer           = C.LEANSIG_NULL_POINTER
	ResultInvalidLength         = C.LEANSIG_INVALID_LENGTH
	ResultSigningFailed         = C.LEANSIG_SIGNING_FAILED
	ResultDeserializationFailed = C.LEANSIG_DESERIALIZATION_FAILED
	ResultVerificationFailed    = C.LEANSIG_VERIFICATION_FAILED
	ResultEpochNotPrepared      = C.LEANSIG_EPOCH_NOT_PREPARED
)

// Keypair wraps an opaque leansig keypair handle.
type Keypair struct {
	ptr *C.LeansigKeypair
}

// GenerateKeypair creates a new XMSS keypair.
//
// Parameters:
//   - seed: random seed for key generation.
//   - activationEpoch: starting epoch for which the key is active.
//   - numActiveEpochs: number of consecutive epochs the key is active for.
func GenerateKeypair(seed uint64, activationEpoch uint64, numActiveEpochs uint64) (*Keypair, error) {
	var ptr *C.LeansigKeypair
	result := C.leansig_keypair_generate(
		C.uint64_t(seed),
		C.uint64_t(activationEpoch),
		C.uint64_t(numActiveEpochs),
		&ptr,
	)
	if result != ResultOK {
		return nil, fmt.Errorf("leansig_keypair_generate failed with code %d", result)
	}
	return &Keypair{ptr: ptr}, nil
}

// RestoreKeypair reconstructs a Keypair from serialized public and secret keys.
// This is used for loading keys from disk.
func RestoreKeypair(pkBytes []byte, skBytes []byte) (*Keypair, error) {
	if len(pkBytes) == 0 {
		return nil, fmt.Errorf("public key bytes are empty")
	}
	if len(skBytes) == 0 {
		return nil, fmt.Errorf("secret key bytes are empty")
	}

	var kpPtr *C.LeansigKeypair
	pkPtr := (*C.uint8_t)(unsafe.Pointer(&pkBytes[0]))
	pkLen := C.size_t(len(pkBytes))
	skPtr := (*C.uint8_t)(unsafe.Pointer(&skBytes[0]))
	skLen := C.size_t(len(skBytes))

	result := C.leansig_keypair_restore(pkPtr, pkLen, skPtr, skLen, &kpPtr)
	if result != ResultOK {
		return nil, fmt.Errorf("leansig_keypair_restore failed with code %d", result)
	}

	return &Keypair{ptr: kpPtr}, nil
}

// Free releases the memory associated with this keypair.
// The keypair must not be used after calling Free.
func (kp *Keypair) Free() {
	if kp.ptr != nil {
		C.leansig_keypair_free(kp.ptr)
		kp.ptr = nil
	}
}

// PublicKeyBytes returns the SSZ-serialized public key.
func (kp *Keypair) PublicKeyBytes() ([]byte, error) {
	if kp.ptr == nil {
		return nil, fmt.Errorf("keypair is nil")
	}
	var data *C.uint8_t
	var dataLen C.size_t
	result := C.leansig_pubkey_serialize(kp.ptr, &data, &dataLen)
	if result != ResultOK {
		return nil, fmt.Errorf("leansig_pubkey_serialize failed with code %d", result)
	}
	// Copy the data to a Go-managed slice
	goBytes := C.GoBytes(unsafe.Pointer(data), C.int(dataLen))
	C.leansig_bytes_free(data, dataLen)
	return goBytes, nil
}

// SecretKeyBytes returns the SSZ-serialized secret key.
func (kp *Keypair) SecretKeyBytes() ([]byte, error) {
	if kp.ptr == nil {
		return nil, fmt.Errorf("keypair is nil")
	}
	var data *C.uint8_t
	var dataLen C.size_t
	result := C.leansig_seckey_serialize(kp.ptr, &data, &dataLen)
	if result != ResultOK {
		return nil, fmt.Errorf("leansig_seckey_serialize failed with code %d", result)
	}
	goBytes := C.GoBytes(unsafe.Pointer(data), C.int(dataLen))
	C.leansig_bytes_free(data, dataLen)
	return goBytes, nil
}

// ActivationStart returns the start of the activation interval.
func (kp *Keypair) ActivationStart() uint64 {
	if kp.ptr == nil {
		return 0
	}
	return uint64(C.leansig_sk_activation_start(kp.ptr))
}

// ActivationEnd returns the end (exclusive) of the activation interval.
func (kp *Keypair) ActivationEnd() uint64 {
	if kp.ptr == nil {
		return 0
	}
	return uint64(C.leansig_sk_activation_end(kp.ptr))
}

// PreparedStart returns the start of the currently prepared signing window.
func (kp *Keypair) PreparedStart() uint64 {
	if kp.ptr == nil {
		return 0
	}
	return uint64(C.leansig_sk_prepared_start(kp.ptr))
}

// PreparedEnd returns the end (exclusive) of the currently prepared signing window.
func (kp *Keypair) PreparedEnd() uint64 {
	if kp.ptr == nil {
		return 0
	}
	return uint64(C.leansig_sk_prepared_end(kp.ptr))
}

// AdvancePreparation advances the secret key's prepared interval to the next window.
func (kp *Keypair) AdvancePreparation() error {
	if kp.ptr == nil {
		return fmt.Errorf("keypair is nil")
	}
	result := C.leansig_sk_advance_preparation(kp.ptr)
	if result != ResultOK {
		return fmt.Errorf("leansig_sk_advance_preparation failed with code %d", result)
	}
	return nil
}

// Sign produces an XMSS signature for a 32-byte message at the given epoch.
// The epoch must be within the key's prepared interval.
// Returns the SSZ-serialized signature bytes.
func (kp *Keypair) Sign(epoch uint32, message [MessageLength]byte) ([]byte, error) {
	if kp.ptr == nil {
		return nil, fmt.Errorf("keypair is nil")
	}
	var sigData *C.uint8_t
	var sigLen C.size_t
	result := C.leansig_sign(
		kp.ptr,
		C.uint32_t(epoch),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		&sigData,
		&sigLen,
	)
	if result != ResultOK {
		return nil, fmt.Errorf("leansig_sign failed with code %d", result)
	}
	goBytes := C.GoBytes(unsafe.Pointer(sigData), C.int(sigLen))
	C.leansig_bytes_free(sigData, sigLen)
	return goBytes, nil
}

// Verify checks an XMSS signature against a serialized public key, epoch, and message.
// Returns nil if the signature is valid, an error otherwise.
func Verify(pubkeyBytes []byte, epoch uint32, message [MessageLength]byte, sigBytes []byte) error {
	if len(pubkeyBytes) == 0 || len(sigBytes) == 0 {
		return fmt.Errorf("empty pubkey or signature bytes")
	}
	result := C.leansig_verify(
		(*C.uint8_t)(unsafe.Pointer(&pubkeyBytes[0])),
		C.size_t(len(pubkeyBytes)),
		C.uint32_t(epoch),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		(*C.uint8_t)(unsafe.Pointer(&sigBytes[0])),
		C.size_t(len(sigBytes)),
	)
	if result == ResultOK {
		return nil
	}
	if result == ResultVerificationFailed {
		return fmt.Errorf("signature verification failed")
	}
	return fmt.Errorf("leansig_verify failed with code %d", result)
}

// VerifyWithKeypair checks an XMSS signature using the public key from a keypair.
// Convenience wrapper that avoids public key serialization/deserialization.
func (kp *Keypair) VerifyWithKeypair(epoch uint32, message [MessageLength]byte, sigBytes []byte) error {
	if kp.ptr == nil {
		return fmt.Errorf("keypair is nil")
	}
	if len(sigBytes) == 0 {
		return fmt.Errorf("empty signature bytes")
	}
	result := C.leansig_verify_with_keypair(
		kp.ptr,
		C.uint32_t(epoch),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		(*C.uint8_t)(unsafe.Pointer(&sigBytes[0])),
		C.size_t(len(sigBytes)),
	)
	if result == ResultOK {
		return nil
	}
	if result == ResultVerificationFailed {
		return fmt.Errorf("signature verification failed")
	}
	return fmt.Errorf("leansig_verify_with_keypair failed with code %d", result)
}
