package sim

import (
	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/crypto/insecure"
)

type insecureCrypto struct{}

// Insecure runs a cluster on crypto/insecure: every proof is checked, but
// keys are derived from validator indices and signatures can be forged. It
// needs no native code and proves instantly.
func Insecure() Crypto { return insecureCrypto{} }

func (insecureCrypto) Scheme() crypto.Scheme { return insecure.Scheme{} }

func (insecureCrypto) PublicKeys(i uint64) (crypto.PublicKey, crypto.PublicKey) {
	return insecure.AttestationPublicKey(i), insecure.ProposalPublicKey(i)
}

func (insecureCrypto) Signer(validatorIDs []uint64) crypto.Signer {
	return insecure.NewKeys(validatorIDs...)
}

func (insecureCrypto) Close() {}
