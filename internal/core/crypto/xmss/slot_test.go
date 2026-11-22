package xmss

import (
	"errors"
	"math"
	"testing"
)

func TestSlot(t *testing.T) {
	if got, err := slot32(math.MaxUint32); err != nil || got != math.MaxUint32 {
		t.Fatalf("slot32(MaxUint32) = %d, %v", got, err)
	}
	if _, err := slot32(math.MaxUint32 + 1); err == nil {
		t.Fatal("slot32(MaxUint32+1) did not overflow")
	}
}

// A slot past 32 bits is refused at every entry point rather than truncated on
// its way into Rust, before any proving starts. One real key makes the inputs
// valid, so only the slot can be the reason.
func TestAPIRefusesSlotsPastUint32(t *testing.T) {
	const slot = math.MaxUint32 + 1
	kp, err := GenerateKeyPair("slot-overflow", 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer kp.Close()
	publicKey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	pubkey, err := ParsePublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	defer FreePublicKey(pubkey)
	sigBytes, err := kp.Sign(1, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	sig, err := ParseSignature(sigBytes[:])
	if err != nil {
		t.Fatal(err)
	}
	defer FreeSignature(sig)
	keys := []CPubKey{pubkey}
	bindings := []MessageBinding{{Slot: slot}}

	calls := map[string]func() error{
		"Sign": func() error { _, err := kp.Sign(slot, [32]byte{}); return err },
		"VerifySignatureSSZ": func() error {
			_, err := VerifySignatureSSZ(publicKey, slot, [32]byte{}, sigBytes)
			return err
		},
		"AggregateWithChildren": func() error {
			_, err := AggregateWithChildren(keys, []CSig{sig}, nil, [32]byte{}, slot)
			return err
		},
		"VerifyAggregatedSignature": func() error { return VerifyAggregatedSignature([]byte{1}, keys, [32]byte{}, slot) },
		"MergeType1Proofs": func() error {
			_, err := MergeType1Proofs(nil, []RawSignature{{Pubkey: pubkey, Signature: sig, Slot: slot}})
			return err
		},
		"SplitType2Proof": func() error {
			_, err := SplitType2Proof([]byte{1}, [][]CPubKey{keys}, bindings, [32]byte{})
			return err
		},
		"VerifyType2Proof": func() error { return VerifyType2Proof([]byte{1}, [][]CPubKey{keys}, bindings) },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrSlotOverflow) {
			t.Errorf("%s: err = %v, want ErrSlotOverflow", name, err)
		}
	}
}
