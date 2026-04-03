package xmss

import (
	"testing"
)

// TestAggregateSignaturesRoundtrip generates keys, signs, serializes signatures
// to SSZ bytes, deserializes back, and aggregates — same path as the engine.
func TestAggregateSignaturesRoundtrip(t *testing.T) {
	// Generate two keypairs.
	kp1, err := GenerateKeyPair("agg-test-validator-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen 0: %v", err)
	}
	defer kp1.Close()

	kp2, err := GenerateKeyPair("agg-test-validator-1", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen 1: %v", err)
	}
	defer kp2.Close()

	// Sign the same message at slot 0.
	var message [32]byte
	message[0] = 0xab

	sig1Bytes, err := kp1.Sign(0, message)
	if err != nil {
		t.Fatalf("sign 0: %v", err)
	}
	sig2Bytes, err := kp2.Sign(0, message)
	if err != nil {
		t.Fatalf("sign 1: %v", err)
	}

	// Verify individually (same path as onGossipAttestation).
	pk1Bytes, _ := kp1.PublicKeyBytes()
	pk2Bytes, _ := kp2.PublicKeyBytes()

	valid, err := VerifySignatureSSZ(pk1Bytes, 0, message, sig1Bytes)
	if err != nil || !valid {
		t.Fatalf("verify sig1: valid=%v err=%v", valid, err)
	}
	valid, err = VerifySignatureSSZ(pk2Bytes, 0, message, sig2Bytes)
	if err != nil || !valid {
		t.Fatalf("verify sig2: valid=%v err=%v", valid, err)
	}

	// Now parse pubkeys and signatures to opaque handles (same path as AggregateCommitteeSignatures).
	cpk1, err := ParsePublicKey(pk1Bytes)
	if err != nil {
		t.Fatalf("parse pk1: %v", err)
	}
	defer FreePublicKey(cpk1)

	cpk2, err := ParsePublicKey(pk2Bytes)
	if err != nil {
		t.Fatalf("parse pk2: %v", err)
	}
	defer FreePublicKey(cpk2)

	csig1, err := ParseSignature(sig1Bytes[:])
	if err != nil {
		t.Fatalf("parse sig1: %v", err)
	}
	defer FreeSignature(csig1)

	csig2, err := ParseSignature(sig2Bytes[:])
	if err != nil {
		t.Fatalf("parse sig2: %v", err)
	}
	defer FreeSignature(csig2)

	// Aggregate.
	EnsureProverReady()

	proofBytes, err := AggregateSignatures(
		[]CPubKey{cpk1, cpk2},
		[]CSig{csig1, csig2},
		message,
		0,
	)
	if err != nil {
		t.Fatalf("aggregate failed: %v", err)
	}
	if len(proofBytes) == 0 {
		t.Fatal("empty proof")
	}

	t.Logf("aggregation succeeded: proof size = %d bytes", len(proofBytes))

	// Verify the aggregated proof.
	EnsureVerifierReady()

	err = VerifyAggregatedSignature(proofBytes, []CPubKey{cpk1, cpk2}, message, 0)
	if err != nil {
		t.Fatalf("verify aggregated failed: %v", err)
	}

	t.Log("aggregated verification succeeded")
}
