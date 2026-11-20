package xmss

import (
	"errors"
	"fmt"
	"unsafe"
)

const (
	MessageLength    = 32
	MaxProofSize     = 1 << 19
	SignatureBuffer  = 4000
	PubkeyBuffer     = 256
	PrivateKeyBuffer = 10 << 20
)

var (
	ErrEmptyInput          = errors.New("empty input")
	ErrCountMismatch       = errors.New("public key count does not match signature count")
	ErrAggregationFailed   = errors.New("signature aggregation failed")
	ErrSerializationFailed = errors.New("proof serialization failed")
	ErrProofTooBig         = errors.New("aggregated proof exceeds 512 KiB")
	ErrVerificationFailed  = errors.New("aggregated signature verification failed")
	ErrDeserializeFailed   = errors.New("proof deserialization failed")
	ErrSigningFailed       = errors.New("signing failed")
	ErrInvalidSignature    = errors.New("signature verification returned invalid")
	ErrSignatureError      = errors.New("signature verification error (malformed data)")
	ErrPubkeyParseFailed   = errors.New("public key parsing failed")
	ErrKeypairParseFailed  = errors.New("keypair parsing failed")
	ErrMalformedChildProof = errors.New("malformed child proof")
	ErrMalformedRawInput   = errors.New("malformed raw signature input")
	ErrSetupFailed         = errors.New("XMSS setup failed")
)

const LogInvRate = 2

type CPubKey = unsafe.Pointer

type CSig = unsafe.Pointer

type ChildProof struct {
	Pubkeys []CPubKey
	Proof   []byte
}

// Type1Input is one Type-1 proof to merge, with the message and slot it proves. Proof
// bytes carry no claims of their own, so the merge has to be told what each one proves.
type Type1Input struct {
	Pubkeys []CPubKey
	Proof   []byte
	Message [MessageLength]byte
	Slot    uint32
}

type MessageBinding struct {
	Message [MessageLength]byte
	Slot    uint32
}

// RawSignature is a signature merged into a Type-2 as it is, without first being proved
// on its own.
type RawSignature struct {
	Pubkey    CPubKey
	Signature CSig
	Message   [MessageLength]byte
	Slot      uint32
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
		if len(child.Proof) == 0 {
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
