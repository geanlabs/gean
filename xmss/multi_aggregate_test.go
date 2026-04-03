package xmss

import (
	"testing"
)

// TestMultipleAggregationsSequential tests calling aggregate multiple times
// with different data — simulates multiple interval 2 ticks.
func TestMultipleAggregationsSequential(t *testing.T) {
	kp1, _ := GenerateKeyPair("multi-agg-0", 0, 1<<18)
	defer kp1.Close()
	kp2, _ := GenerateKeyPair("multi-agg-1", 0, 1<<18)
	defer kp2.Close()

	pk1, _ := kp1.PublicKeyBytes()
	pk2, _ := kp2.PublicKeyBytes()

	EnsureProverReady()
	EnsureVerifierReady()

	for slot := uint32(0); slot < 5; slot++ {
		var message [32]byte
		message[0] = byte(slot)
		message[1] = 0xab

		sig1, err := kp1.Sign(slot, message)
		if err != nil {
			t.Fatalf("slot %d sign kp1: %v", slot, err)
		}
		sig2, err := kp2.Sign(slot, message)
		if err != nil {
			t.Fatalf("slot %d sign kp2: %v", slot, err)
		}

		// Parse from SSZ bytes (simulating P2P round-trip)
		csig1, _ := ParseSignature(sig1[:])
		defer FreeSignature(csig1)
		csig2, _ := ParseSignature(sig2[:])
		defer FreeSignature(csig2)

		cpk1, _ := ParsePublicKey(pk1)
		defer FreePublicKey(cpk1)
		cpk2, _ := ParsePublicKey(pk2)
		defer FreePublicKey(cpk2)

		proof, err := AggregateSignatures(
			[]CPubKey{cpk1, cpk2},
			[]CSig{csig1, csig2},
			message,
			slot,
		)
		if err != nil {
			t.Fatalf("slot %d aggregate FAILED: %v", slot, err)
		}

		err = VerifyAggregatedSignature(proof, []CPubKey{cpk1, cpk2}, message, slot)
		if err != nil {
			t.Fatalf("slot %d verify FAILED: %v", slot, err)
		}

		t.Logf("slot %d: aggregate + verify OK (proof=%d bytes)", slot, len(proof))
	}
}
