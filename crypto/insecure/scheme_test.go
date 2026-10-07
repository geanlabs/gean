package insecure

import (
	"testing"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/crypto/schemetest"
)

func TestSchemeConformance(t *testing.T) {
	schemetest.Run(t, Scheme{}, func(t *testing.T, i int) (crypto.PublicKey, func(uint32, [32]byte) crypto.Signature) {
		pk := AttestationPublicKey(uint64(i))
		return pk, func(slot uint32, message [32]byte) crypto.Signature { return sign(pk, slot, message) }
	})
}

// A forged child proof must not be folded into an aggregate, or the result
// would claim signers who never signed.
func TestAggregateRejectsForgedChild(t *testing.T) {
	message := [32]byte{0x01}
	forged := crypto.Proof{PublicKeys: []crypto.PublicKey{AttestationPublicKey(9)}, Proof: make([]byte, proofLen)}
	if _, err := (Scheme{}).Aggregate(nil, []crypto.Proof{forged}, message, 1); err == nil {
		t.Fatal("aggregate accepted a forged child proof")
	}
}
