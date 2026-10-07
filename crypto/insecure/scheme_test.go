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

// A proof must commit to its exact signers: one over [A] must not verify as
// one over [A, A], and an aggregate must not count a signer twice.
func TestProofsRejectDuplicateSigners(t *testing.T) {
	message := [32]byte{0x02}
	a := AttestationPublicKey(0)
	raw := crypto.RawSignature{PublicKey: a, Signature: sign(a, 1, message)}
	single, err := (Scheme{}).Aggregate([]crypto.RawSignature{raw}, nil, message, 1)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if err := (Scheme{}).VerifyAggregate(single, []crypto.PublicKey{a, a}, message, 1); err == nil {
		t.Fatal("proof over one signer verified against a duplicated key list")
	}
	if _, err := (Scheme{}).Aggregate([]crypto.RawSignature{raw, raw}, nil, message, 1); err == nil {
		t.Fatal("aggregate counted a signer twice")
	}
}
