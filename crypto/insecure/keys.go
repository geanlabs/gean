package insecure

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/types"
)

// Keys signs as a set of validators with insecure keys derived from their
// indices. It implements crypto.Signer, signing what xmss.KeyManager signs.
type Keys struct {
	ids []uint64
}

var _ crypto.Signer = (*Keys)(nil)

func NewKeys(validatorIDs ...uint64) *Keys {
	ids := slices.Clone(validatorIDs)
	slices.Sort(ids)
	return &Keys{ids: ids}
}

// AttestationPublicKey is validator i's attestation key.
func AttestationPublicKey(i uint64) crypto.PublicKey { return derive("attestation", i) }

// ProposalPublicKey is validator i's proposal key.
func ProposalPublicKey(i uint64) crypto.PublicKey { return derive("proposal", i) }

func (k *Keys) ValidatorIDs() []uint64 { return slices.Clone(k.ids) }

func (k *Keys) SignAttestation(validatorID uint64, data *types.AttestationData) (crypto.Signature, error) {
	if !slices.Contains(k.ids, validatorID) {
		return crypto.Signature{}, fmt.Errorf("attestation key for validator %d: %w", validatorID, crypto.ErrNoKey)
	}
	if data == nil {
		return crypto.Signature{}, fmt.Errorf("attestation data is nil")
	}
	root, err := data.HashTreeRoot()
	if err != nil {
		return crypto.Signature{}, fmt.Errorf("hash tree root failed: %w", err)
	}
	slot, err := slot32(data.Slot)
	if err != nil {
		return crypto.Signature{}, err
	}
	return sign(AttestationPublicKey(validatorID), slot, root), nil
}

func (k *Keys) SignBlock(validatorID, slot uint64, blockRoot [32]byte) (crypto.Signature, error) {
	if !slices.Contains(k.ids, validatorID) {
		return crypto.Signature{}, fmt.Errorf("proposal key for validator %d: %w", validatorID, crypto.ErrNoKey)
	}
	s, err := slot32(slot)
	if err != nil {
		return crypto.Signature{}, err
	}
	return sign(ProposalPublicKey(validatorID), s, blockRoot), nil
}

func derive(kind string, i uint64) crypto.PublicKey {
	return sha256.Sum256(binary.BigEndian.AppendUint64([]byte("gean-insecure-"+kind+"-key"), i))
}

func slot32(slot uint64) (uint32, error) {
	s := uint32(slot)
	if uint64(s) != slot {
		return 0, fmt.Errorf("slot %d overflows uint32", slot)
	}
	return s, nil
}
