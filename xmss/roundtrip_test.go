package xmss

import (
	"testing"
)

// TestSignatureSSZRoundtripThenAggregate tests the exact path that fails in production:
// Generate key → sign → serialize to [3112]byte → deserialize back → aggregate.
// This simulates what happens when a proposer attestation arrives via P2P.
func TestSignatureSSZRoundtripThenAggregate(t *testing.T) {
	kp, err := GenerateKeyPair("roundtrip-test-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	var message [32]byte
	message[0] = 0xab

	// Sign → get [3112]byte (SSZ serialized via hashsig_signature_to_bytes)
	sigBytes, err := kp.Sign(0, message)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Parse pubkey
	pkBytes, _ := kp.PublicKeyBytes()
	cpk, err := ParsePublicKey(pkBytes)
	if err != nil {
		t.Fatalf("parse pk: %v", err)
	}
	defer FreePublicKey(cpk)

	// NOW: simulate SSZ round-trip by parsing from the serialized bytes.
	// This is what happens when processProposerAttestation receives a block from P2P.
	csig, err := ParseSignature(sigBytes[:])
	if err != nil {
		t.Fatalf("parse sig from SSZ bytes: %v", err)
	}
	defer FreeSignature(csig)

	// Aggregate with the round-tripped signature.
	EnsureProverReady()
	proofBytes, err := AggregateSignatures(
		[]CPubKey{cpk},
		[]CSig{csig},
		message,
		0,
	)
	if err != nil {
		t.Fatalf("aggregate with SSZ-round-tripped sig FAILED: %v", err)
	}
	t.Logf("aggregate with SSZ-round-tripped sig succeeded: proof=%d bytes", len(proofBytes))

	// Verify
	EnsureVerifierReady()
	err = VerifyAggregatedSignature(proofBytes, []CPubKey{cpk}, message, 0)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	t.Log("verification succeeded")
}
