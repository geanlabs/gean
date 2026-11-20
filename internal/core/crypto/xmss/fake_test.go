//go:build fakecrypto

package xmss

import (
	"testing"
)

// A key written by keygen loads back with the same public key, which is what
// the node checks against the registry.
func TestFakeKeyRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair("validator 0", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := kp.PrivateKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := KeyPairFromSecret(secret, 0)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := kp.PublicKeyBytes()
	if got, _ := loaded.PublicKeyBytes(); got != want {
		t.Fatalf("loaded pubkey %x, want %x", got, want)
	}
}

func TestFakeSignAndVerify(t *testing.T) {
	kp, _ := GenerateKeyPair("validator 0", 0, 1)
	msg := [32]byte{1}
	sig, err := kp.Sign(3, msg)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := kp.PublicKeyBytes()
	if ok, err := VerifySignatureSSZ(pub, 3, msg, sig); !ok || err != nil {
		t.Fatalf("verify=%v err=%v", ok, err)
	}
	if other, _ := kp.Sign(4, msg); other == sig {
		t.Fatal("signatures for different slots are equal")
	}

	kp.Close()
	if _, err := kp.Sign(3, msg); err == nil {
		t.Fatal("a closed key signed")
	}
}

// A zero-value key pair holds no key, as in the real build, where its native
// handle is nil, so it must not sign.
func TestFakeZeroKeyPairCannotSign(t *testing.T) {
	var kp ValidatorKeyPair
	if _, err := kp.Sign(1, [32]byte{1}); err == nil {
		t.Fatal("a zero-value key pair signed")
	}
	if _, err := kp.PublicKeyBytes(); err == nil {
		t.Fatal("a zero-value key pair has a public key")
	}
}

// The fake skips the cryptography, not the input checks.
func TestFakeRejectsMalformedInput(t *testing.T) {
	msg := [32]byte{1}
	if _, err := AggregateWithChildren(nil, nil, nil, msg, 1); err != ErrEmptyInput {
		t.Fatalf("empty aggregate err=%v, want ErrEmptyInput", err)
	}
	if err := VerifyAggregatedSignature(nil, nil, msg, 1); err != ErrEmptyInput {
		t.Fatalf("empty verify err=%v, want ErrEmptyInput", err)
	}
	if err := VerifyType2Proof([]byte{1}, [][]CPubKey{{}}, nil); err != ErrCountMismatch {
		t.Fatalf("mismatched type-2 err=%v, want ErrCountMismatch", err)
	}

	kp, _ := GenerateKeyPair("validator 0", 0, 1)
	pk := kp.PublicKey()
	sig, _ := kp.Sign(1, msg)
	parsed, err := ParseSignature(sig[:])
	if err != nil {
		t.Fatal(err)
	}
	proof, err := AggregateWithChildren([]CPubKey{pk}, []CSig{parsed}, nil, msg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyAggregatedSignature(proof, []CPubKey{pk}, msg, 1); err != nil {
		t.Fatalf("verify aggregate: %v", err)
	}
}
