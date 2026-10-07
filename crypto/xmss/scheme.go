package xmss

import (
	"fmt"

	"github.com/geanlabs/gean/crypto"
)

// Scheme is the XMSS implementation of crypto.Scheme. It parses each public
// key once and keeps the handle for the life of the scheme; signatures are
// parsed per call and freed before it returns.
type Scheme struct {
	keys *PubKeyCache
}

var _ crypto.Scheme = (*Scheme)(nil)

func NewScheme() *Scheme {
	return &Scheme{keys: NewPubKeyCache()}
}

// Close frees every cached public key. The scheme must not be used after.
func (s *Scheme) Close() {
	s.keys.Close()
}

func (s *Scheme) VerifySignature(pubkey crypto.PublicKey, slot uint32, message [32]byte, sig crypto.Signature) error {
	valid, err := VerifySignatureSSZ(pubkey, slot, message, sig)
	if err != nil {
		return err
	}
	if !valid {
		return crypto.ErrInvalidSignature
	}
	return nil
}

func (s *Scheme) Aggregate(raw []crypto.RawSignature, children []crypto.Proof, message [32]byte, slot uint32) ([]byte, error) {
	pubkeys := make([]CPubKey, 0, len(raw))
	sigs := make([]CSig, 0, len(raw))
	defer func() {
		for _, sig := range sigs {
			FreeSignature(sig)
		}
	}()
	for i, r := range raw {
		pk, err := s.keys.Get(r.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("raw signer %d public key: %w", i, err)
		}
		sig, err := ParseSignature(r.Signature[:])
		if err != nil {
			return nil, fmt.Errorf("raw signer %d signature: %w", i, err)
		}
		pubkeys = append(pubkeys, pk)
		sigs = append(sigs, sig)
	}
	childProofs := make([]ChildProof, 0, len(children))
	for i, child := range children {
		keys, err := s.handles(child.PublicKeys)
		if err != nil {
			return nil, fmt.Errorf("child proof %d: %w", i, err)
		}
		childProofs = append(childProofs, ChildProof{Pubkeys: keys, Proof: child.Proof})
	}
	return AggregateWithChildren(pubkeys, sigs, childProofs, message, slot)
}

func (s *Scheme) VerifyAggregate(proof []byte, pubkeys []crypto.PublicKey, message [32]byte, slot uint32) error {
	keys, err := s.handles(pubkeys)
	if err != nil {
		return err
	}
	return VerifyAggregatedSignature(proof, keys, message, slot)
}

func (s *Scheme) MergeBlockProof(inputs []crypto.Proof) ([]byte, error) {
	type1 := make([]Type1Input, 0, len(inputs))
	for i, input := range inputs {
		keys, err := s.handles(input.PublicKeys)
		if err != nil {
			return nil, fmt.Errorf("block proof input %d: %w", i, err)
		}
		type1 = append(type1, Type1Input{Pubkeys: keys, Proof: input.Proof})
	}
	return MergeType1Proofs(type1)
}

func (s *Scheme) VerifyBlockProof(proof []byte, pubkeys [][]crypto.PublicKey, bindings []crypto.Binding) error {
	groups, err := s.groupHandles(pubkeys)
	if err != nil {
		return err
	}
	xmssBindings := make([]MessageBinding, len(bindings))
	for i, b := range bindings {
		xmssBindings[i] = MessageBinding{Message: b.Message, Slot: b.Slot}
	}
	return VerifyType2Proof(proof, groups, xmssBindings)
}

func (s *Scheme) SplitBlockProof(proof []byte, pubkeys [][]crypto.PublicKey, message [32]byte) ([]byte, error) {
	groups, err := s.groupHandles(pubkeys)
	if err != nil {
		return nil, err
	}
	return SplitType2Proof(proof, groups, message)
}

func (s *Scheme) handles(pubkeys []crypto.PublicKey) ([]CPubKey, error) {
	keys := make([]CPubKey, len(pubkeys))
	for i, pubkey := range pubkeys {
		pk, err := s.keys.Get(pubkey)
		if err != nil {
			return nil, fmt.Errorf("public key %d: %w", i, err)
		}
		keys[i] = pk
	}
	return keys, nil
}

func (s *Scheme) groupHandles(groups [][]crypto.PublicKey) ([][]CPubKey, error) {
	out := make([][]CPubKey, len(groups))
	for i, group := range groups {
		keys, err := s.handles(group)
		if err != nil {
			return nil, fmt.Errorf("group %d: %w", i, err)
		}
		out[i] = keys
	}
	return out, nil
}
