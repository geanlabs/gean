// Package xmsssim runs simulated clusters on real XMSS keys and proofs, for
// scenarios that must exercise the post-quantum scheme the node runs.
package xmsssim

import (
	"fmt"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/sim"
)

// keyLifetime is how many slots a simulated validator key can sign for. It is
// far below a real key's lifetime, so generating keys takes seconds.
const keyLifetime = 1 << 10

// Crypto is a sim.Crypto over XMSS. Validator i's keys are derived from i, so
// every run uses the same keys.
type Crypto struct {
	scheme      *xmss.Scheme
	attestation []*xmss.ValidatorKeyPair
	proposal    []*xmss.ValidatorKeyPair
	attPubkeys  []crypto.PublicKey
	propPubkeys []crypto.PublicKey
}

var _ sim.Crypto = (*Crypto)(nil)

// New generates keys for validators 0..count-1.
func New(count int) (*Crypto, error) {
	c := &Crypto{scheme: xmss.NewScheme()}
	for i := range uint64(count) {
		att, attPub, err := keyPair(fmt.Sprintf("gean-sim-validator-%d-attestation", i), i)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.attestation, c.attPubkeys = append(c.attestation, att), append(c.attPubkeys, attPub)
		prop, propPub, err := keyPair(fmt.Sprintf("gean-sim-validator-%d-proposal", i), i)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.proposal, c.propPubkeys = append(c.proposal, prop), append(c.propPubkeys, propPub)
	}
	return c, nil
}

func (c *Crypto) Scheme() crypto.Scheme { return c.scheme }

func (c *Crypto) PublicKeys(i uint64) (crypto.PublicKey, crypto.PublicKey) {
	return c.attPubkeys[i], c.propPubkeys[i]
}

// Signer returns a key manager over the validators' keys. The keys stay owned
// by c, so the manager must not be closed.
func (c *Crypto) Signer(validatorIDs []uint64) crypto.Signer {
	att := make(map[uint64]*xmss.ValidatorKeyPair, len(validatorIDs))
	prop := make(map[uint64]*xmss.ValidatorKeyPair, len(validatorIDs))
	for _, id := range validatorIDs {
		att[id], prop[id] = c.attestation[id], c.proposal[id]
	}
	return xmss.NewKeyManager(att, prop)
}

func (c *Crypto) Close() {
	for _, kp := range c.attestation {
		kp.Close()
	}
	for _, kp := range c.proposal {
		kp.Close()
	}
	c.scheme.Close()
}

func keyPair(seed string, index uint64) (*xmss.ValidatorKeyPair, crypto.PublicKey, error) {
	kp, err := xmss.GenerateKeyPair(seed, 0, keyLifetime)
	if err != nil {
		return nil, crypto.PublicKey{}, fmt.Errorf("generate key %s: %w", seed, err)
	}
	kp.Index = index
	pk, err := kp.PublicKeyBytes()
	if err != nil {
		kp.Close()
		return nil, crypto.PublicKey{}, fmt.Errorf("public key %s: %w", seed, err)
	}
	return kp, pk, nil
}
