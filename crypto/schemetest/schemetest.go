// Package schemetest is the conformance suite every crypto.Scheme must pass,
// so the schemes stay interchangeable.
package schemetest

import (
	"testing"

	"github.com/geanlabs/gean/crypto"
)

// SignerFunc returns validator i's public key and a function that signs a
// message at a slot with it.
type SignerFunc func(t *testing.T, i int) (crypto.PublicKey, func(slot uint32, message [32]byte) crypto.Signature)

// Run checks scheme against the crypto.Scheme contract using four signers,
// each signing at most one message per slot as one-time keys require.
func Run(t *testing.T, scheme crypto.Scheme, signer SignerFunc) {
	t.Helper()
	const slot = 7
	message := [32]byte{0x11}
	other := [32]byte{0x22}

	keys := make([]crypto.PublicKey, 3)
	raw := make([]crypto.RawSignature, 3)
	for i := range keys {
		pk, sign := signer(t, i)
		keys[i] = pk
		raw[i] = crypto.RawSignature{PublicKey: pk, Signature: sign(slot, message)}
	}

	t.Run("signature", func(t *testing.T) {
		if err := scheme.VerifySignature(keys[0], slot, message, raw[0].Signature); err != nil {
			t.Fatalf("valid signature rejected: %v", err)
		}
		if err := scheme.VerifySignature(keys[1], slot, message, raw[0].Signature); err == nil {
			t.Fatal("signature verified under another key")
		}
		if err := scheme.VerifySignature(keys[0], slot, other, raw[0].Signature); err == nil {
			t.Fatal("signature verified for another message")
		}
	})

	child, err := scheme.Aggregate(raw[:2], nil, message, slot)
	if err != nil {
		t.Fatalf("aggregate raw signatures: %v", err)
	}

	t.Run("aggregate", func(t *testing.T) {
		if err := scheme.VerifyAggregate(child, keys[:2], message, slot); err != nil {
			t.Fatalf("valid aggregate rejected: %v", err)
		}
		if err := scheme.VerifyAggregate(child, keys, message, slot); err == nil {
			t.Fatal("aggregate verified for a signer it does not cover")
		}
		if err := scheme.VerifyAggregate(child, keys[:2], other, slot); err == nil {
			t.Fatal("aggregate verified for another message")
		}
	})

	t.Run("lone child", func(t *testing.T) {
		if _, err := scheme.Aggregate(nil, []crypto.Proof{{PublicKeys: keys[:2], Proof: child}}, message, slot); err == nil {
			t.Fatal("aggregated a single child proof with no raw signatures")
		}
	})

	t.Run("aggregate with child", func(t *testing.T) {
		merged, err := scheme.Aggregate(raw[2:], []crypto.Proof{{PublicKeys: keys[:2], Proof: child}}, message, slot)
		if err != nil {
			t.Fatalf("aggregate with child: %v", err)
		}
		if err := scheme.VerifyAggregate(merged, keys, message, slot); err != nil {
			t.Fatalf("aggregate with child rejected: %v", err)
		}
	})

	t.Run("block proof", func(t *testing.T) {
		proposerKey, signProposer := signer(t, 3)
		proposer, err := scheme.Aggregate([]crypto.RawSignature{{PublicKey: proposerKey, Signature: signProposer(slot, other)}}, nil, other, slot)
		if err != nil {
			t.Fatalf("aggregate proposer signature: %v", err)
		}
		block, err := scheme.MergeBlockProof([]crypto.Proof{
			{PublicKeys: keys[:2], Proof: child},
			{PublicKeys: []crypto.PublicKey{proposerKey}, Proof: proposer},
		})
		if err != nil {
			t.Fatalf("merge block proof: %v", err)
		}
		groups := [][]crypto.PublicKey{keys[:2], {proposerKey}}
		bindings := []crypto.Binding{{Message: message, Slot: slot}, {Message: other, Slot: slot}}
		if err := scheme.VerifyBlockProof(block, groups, bindings); err != nil {
			t.Fatalf("valid block proof rejected: %v", err)
		}
		swapped := []crypto.Binding{bindings[1], bindings[0]}
		if err := scheme.VerifyBlockProof(block, groups, swapped); err == nil {
			t.Fatal("block proof verified against the wrong bindings")
		}

		split, err := scheme.SplitBlockProof(block, groups, message)
		if err != nil {
			t.Fatalf("split block proof: %v", err)
		}
		if err := scheme.VerifyAggregate(split, keys[:2], message, slot); err != nil {
			t.Fatalf("split proof rejected: %v", err)
		}
	})
}
