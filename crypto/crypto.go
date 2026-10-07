// Package crypto is the signature scheme contract consensus code signs, proves
// and verifies with. It speaks only bytes, so consensus logic never holds a
// native handle and the scheme behind it can be replaced: crypto/xmss is the
// post-quantum scheme the node runs, and crypto/insecure is a fast, forgeable
// scheme for simulations and tests.
package crypto

import (
	"errors"

	"github.com/geanlabs/gean/types"
)

// PublicKey is a serialized validator public key.
type PublicKey = [types.PubkeySize]byte

// Signature is a serialized single-validator signature.
type Signature = [types.SignatureSize]byte

// ErrInvalidSignature reports a signature that is well formed but does not
// verify.
var ErrInvalidSignature = errors.New("invalid signature")

// ErrNoKey reports that a Signer holds no key for the requested validator, so
// nothing was signed.
var ErrNoKey = errors.New("no key for validator")

// RawSignature is one validator's signature, with the key that verifies it.
type RawSignature struct {
	PublicKey PublicKey
	Signature Signature
}

// Proof is an aggregate proof with the public keys of everyone it covers.
type Proof struct {
	PublicKeys []PublicKey
	Proof      []byte
}

// Binding is the message and slot one component of a block proof signs.
type Binding struct {
	Message [32]byte
	Slot    uint32
}

// Scheme proves and verifies signatures and aggregate proofs. A
// single-message proof covers one message signed by many validators; a block
// proof merges several single-message proofs, one per binding.
type Scheme interface {
	// VerifySignature returns ErrInvalidSignature if sig is not pubkey's
	// signature of message at slot.
	VerifySignature(pubkey PublicKey, slot uint32, message [32]byte, sig Signature) error
	// Aggregate proves that every raw signer, and everyone the children
	// cover, signed message at slot.
	Aggregate(raw []RawSignature, children []Proof, message [32]byte, slot uint32) ([]byte, error)
	VerifyAggregate(proof []byte, pubkeys []PublicKey, message [32]byte, slot uint32) error
	// MergeBlockProof merges single-message proofs into one block proof.
	MergeBlockProof(inputs []Proof) ([]byte, error)
	// VerifyBlockProof checks a block proof whose i-th component covers
	// pubkeys[i] signing bindings[i].
	VerifyBlockProof(proof []byte, pubkeys [][]PublicKey, bindings []Binding) error
	// SplitBlockProof extracts the single-message proof for message from a
	// block proof whose components cover pubkeys.
	SplitBlockProof(proof []byte, pubkeys [][]PublicKey, message [32]byte) ([]byte, error)
}

// Signer signs this node's validator duties. A missing key is reported as
// ErrNoKey, distinct from a failed signing attempt.
type Signer interface {
	ValidatorIDs() []uint64
	SignAttestation(validatorID uint64, data *types.AttestationData) (Signature, error)
	SignBlock(validatorID, slot uint64, blockRoot [32]byte) (Signature, error)
}
