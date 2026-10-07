// Package insecure is a fast, deterministic crypto.Scheme for simulations and
// tests. It checks every proof against the keys, messages and slots it claims,
// so consensus logic built on it is exercised for real, but anyone can forge
// its signatures: it must never secure a real network. The layering rules keep
// it out of the gean binaries.
package insecure

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/geanlabs/gean/crypto"
)

var (
	errMalformedProof = errors.New("malformed proof")
	errProofMismatch  = errors.New("proof does not match its keys, message or slot")
	errNoComponent    = errors.New("block proof has no component for the message")
)

// Scheme is the insecure crypto.Scheme. A signature is a hash of the key,
// slot and message. A single-message proof is the message and slot in the
// clear followed by a hash committing to them and the signers' keys; a block
// proof is a count followed by its single-message proofs.
type Scheme struct{}

var _ crypto.Scheme = Scheme{}

// proofLen is the size of a single-message proof: message, slot, commitment.
const proofLen = 32 + 4 + sha256.Size

func (Scheme) VerifySignature(pubkey crypto.PublicKey, slot uint32, message [32]byte, sig crypto.Signature) error {
	if sig != sign(pubkey, slot, message) {
		return crypto.ErrInvalidSignature
	}
	return nil
}

func (s Scheme) Aggregate(raw []crypto.RawSignature, children []crypto.Proof, message [32]byte, slot uint32) ([]byte, error) {
	if len(raw)+len(children) == 0 {
		return nil, errors.New("nothing to aggregate")
	}
	var signers []crypto.PublicKey
	for i, r := range raw {
		if err := s.VerifySignature(r.PublicKey, slot, message, r.Signature); err != nil {
			return nil, fmt.Errorf("raw signer %d: %w", i, err)
		}
		signers = append(signers, r.PublicKey)
	}
	for i, child := range children {
		if err := s.VerifyAggregate(child.Proof, child.PublicKeys, message, slot); err != nil {
			return nil, fmt.Errorf("child proof %d: %w", i, err)
		}
		signers = append(signers, child.PublicKeys...)
	}
	return singleProof(signers, message, slot), nil
}

func (Scheme) VerifyAggregate(proof []byte, pubkeys []crypto.PublicKey, message [32]byte, slot uint32) error {
	if len(proof) != proofLen {
		return errMalformedProof
	}
	if !bytes.Equal(proof, singleProof(pubkeys, message, slot)) {
		return errProofMismatch
	}
	return nil
}

func (s Scheme) MergeBlockProof(inputs []crypto.Proof) ([]byte, error) {
	if len(inputs) == 0 {
		return nil, errors.New("nothing to merge")
	}
	merged := binary.BigEndian.AppendUint32(nil, uint32(len(inputs)))
	for i, input := range inputs {
		if len(input.Proof) != proofLen {
			return nil, fmt.Errorf("block proof input %d: %w", i, errMalformedProof)
		}
		message, slot := proofBinding(input.Proof)
		if err := s.VerifyAggregate(input.Proof, input.PublicKeys, message, slot); err != nil {
			return nil, fmt.Errorf("block proof input %d: %w", i, err)
		}
		merged = append(merged, input.Proof...)
	}
	return merged, nil
}

func (s Scheme) VerifyBlockProof(proof []byte, pubkeys [][]crypto.PublicKey, bindings []crypto.Binding) error {
	components, err := blockComponents(proof)
	if err != nil {
		return err
	}
	if len(components) != len(pubkeys) || len(components) != len(bindings) {
		return fmt.Errorf("%w: %d components for %d key groups and %d bindings",
			errProofMismatch, len(components), len(pubkeys), len(bindings))
	}
	for i, component := range components {
		if err := s.VerifyAggregate(component, pubkeys[i], bindings[i].Message, bindings[i].Slot); err != nil {
			return fmt.Errorf("component %d: %w", i, err)
		}
	}
	return nil
}

func (s Scheme) SplitBlockProof(proof []byte, pubkeys [][]crypto.PublicKey, message [32]byte) ([]byte, error) {
	components, err := blockComponents(proof)
	if err != nil {
		return nil, err
	}
	if len(components) != len(pubkeys) {
		return nil, fmt.Errorf("%w: %d components for %d key groups", errProofMismatch, len(components), len(pubkeys))
	}
	for i, component := range components {
		componentMessage, slot := proofBinding(component)
		if componentMessage != message {
			continue
		}
		if err := s.VerifyAggregate(component, pubkeys[i], message, slot); err != nil {
			return nil, fmt.Errorf("component %d: %w", i, err)
		}
		return bytes.Clone(component), nil
	}
	return nil, errNoComponent
}

func sign(pubkey crypto.PublicKey, slot uint32, message [32]byte) crypto.Signature {
	h := sha256.New()
	h.Write([]byte("gean-insecure-signature"))
	h.Write(pubkey[:])
	h.Write(binary.BigEndian.AppendUint32(nil, slot))
	h.Write(message[:])
	var sig crypto.Signature
	copy(sig[:], h.Sum(nil))
	return sig
}

// singleProof commits to the signer set regardless of order.
func singleProof(pubkeys []crypto.PublicKey, message [32]byte, slot uint32) []byte {
	sorted := slices.Clone(pubkeys)
	slices.SortFunc(sorted, func(a, b crypto.PublicKey) int { return bytes.Compare(a[:], b[:]) })
	sorted = slices.Compact(sorted)

	h := sha256.New()
	h.Write([]byte("gean-insecure-proof"))
	h.Write(message[:])
	h.Write(binary.BigEndian.AppendUint32(nil, slot))
	for _, pk := range sorted {
		h.Write(pk[:])
	}
	proof := append(message[:], binary.BigEndian.AppendUint32(nil, slot)...)
	return h.Sum(proof)
}

func proofBinding(proof []byte) ([32]byte, uint32) {
	var message [32]byte
	copy(message[:], proof[:32])
	return message, binary.BigEndian.Uint32(proof[32:36])
}

func blockComponents(proof []byte) ([][]byte, error) {
	if len(proof) < 4 {
		return nil, errMalformedProof
	}
	count := int(binary.BigEndian.Uint32(proof[:4]))
	body := proof[4:]
	if count == 0 || len(body) != count*proofLen {
		return nil, errMalformedProof
	}
	components := make([][]byte, count)
	for i := range components {
		components[i] = body[i*proofLen : (i+1)*proofLen]
	}
	return components, nil
}
