//go:build !fakecrypto

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
	"unsafe"
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

func (kp *ValidatorKeyPair) Sign(slot uint32, message [32]byte) ([SignatureSize]byte, error) {
	var result [SignatureSize]byte
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
	if n == 0 || int(n) != SignatureSize {
		return result, fmt.Errorf("signature serialization failed: wrote %d bytes, expected %d", n, SignatureSize)
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

// loaded reports whether kp holds a key that has not been closed.
func (kp *ValidatorKeyPair) loaded() bool {
	return kp != nil && kp.handle != nil
}

// keyPairFromSecret builds the key pair for a validator's serialized secret
// key.
func keyPairFromSecret(skBytes []byte, index uint64) (*ValidatorKeyPair, error) {
	handle := C.hashsig_keypair_from_secret_key((*C.uint8_t)(unsafe.Pointer(&skBytes[0])), C.size_t(len(skBytes)))
	if handle == nil {
		return nil, fmt.Errorf("%w: validator %d", ErrKeypairParseFailed, index)
	}

	return &ValidatorKeyPair{handle: handle, Index: index}, nil
}
