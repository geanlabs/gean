package xmss

import (
	"errors"
	"strings"
	"testing"
)

func TestType2Roundtrip(t *testing.T) {
	key, err := GenerateKeyPair("type-2-roundtrip", 0, 1<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	pubkeyBytes, err := key.PublicKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	pubkey, err := ParsePublicKey(pubkeyBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer FreePublicKey(pubkey)

	var messages [2][32]byte
	messages[0][0] = 1
	messages[1][0] = 2
	inputs := make([]Type1Input, 0, len(messages))
	for slot, message := range messages {
		raw, err := key.Sign(uint32(slot), message)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := ParseSignature(raw[:])
		if err != nil {
			t.Fatal(err)
		}
		proof, err := AggregateSignatures(
			[]CPubKey{pubkey},
			[]CSig{signature},
			message,
			uint32(slot),
		)
		FreeSignature(signature)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, Type1Input{Pubkeys: []CPubKey{pubkey}, Proof: proof, Message: message, Slot: uint32(slot)})
	}

	proof, err := MergeType1Proofs(inputs)
	if err != nil {
		t.Fatal(err)
	}
	groups := [][]CPubKey{{pubkey}, {pubkey}}
	bindings := []MessageBinding{
		{Message: messages[0], Slot: 0},
		{Message: messages[1], Slot: 1},
	}
	if err := VerifyType2Proof(proof, groups, bindings); err != nil {
		t.Fatal(err)
	}
	recovered, err := SplitType2Proof(proof, groups, bindings, messages[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyAggregatedSignature(recovered, []CPubKey{pubkey}, messages[0], 0); err != nil {
		t.Fatal(err)
	}
}

// A proof carries at most one message per slot, so two validators that signed different
// messages at the same slot cannot share a Type-2. The merge has to refuse rather than
// produce a proof no verifier would accept.
func TestType2RejectsTwoMessagesAtOneSlot(t *testing.T) {
	const slot = 5
	var inputs []Type1Input
	for i, seed := range []string{"same-slot-a", "same-slot-b"} {
		key, err := GenerateKeyPair(seed, 0, 1<<10)
		if err != nil {
			t.Fatal(err)
		}
		defer key.Close()
		pubkeyBytes, err := key.PublicKeyBytes()
		if err != nil {
			t.Fatal(err)
		}
		pubkey, err := ParsePublicKey(pubkeyBytes)
		if err != nil {
			t.Fatal(err)
		}
		defer FreePublicKey(pubkey)

		var message [32]byte
		message[0] = byte(i + 1)
		raw, err := key.Sign(slot, message)
		if err != nil {
			t.Fatal(err)
		}
		signature, err := ParseSignature(raw[:])
		if err != nil {
			t.Fatal(err)
		}
		proof, err := AggregateSignatures([]CPubKey{pubkey}, []CSig{signature}, message, slot)
		FreeSignature(signature)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, Type1Input{Pubkeys: []CPubKey{pubkey}, Proof: proof, Message: message, Slot: slot})
	}

	_, err := MergeType1Proofs(inputs)
	if err == nil {
		t.Fatal("merged two different messages at one slot")
	}
	if !errors.Is(err, ErrAggregationFailed) || !strings.Contains(err.Error(), "two different messages at one slot") {
		t.Fatalf("merge failed without naming the cause: %v", err)
	}
}
