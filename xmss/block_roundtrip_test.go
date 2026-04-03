package xmss

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// TestProposerSigThroughBlockSSZ simulates the exact P2P path:
// Node1 signs → builds SignedBlockWithAttestation → SSZ marshal → SSZ unmarshal →
// extract ProposerSignature → ParseSignature → aggregate.
func TestProposerSigThroughBlockSSZ(t *testing.T) {
	kp, err := GenerateKeyPair("block-roundtrip-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	pkBytes, _ := kp.PublicKeyBytes()

	// Sign a message (attestation data root)
	var attDataRoot [32]byte
	attDataRoot[0] = 0xab
	attDataRoot[1] = 0xcd

	sig, err := kp.Sign(1, attDataRoot) // slot 1
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Build a SignedBlockWithAttestation with this signature as ProposerSignature
	signedBlock := &types.SignedBlockWithAttestation{
		Block: &types.BlockWithAttestation{
			Block: &types.Block{
				Slot:          1,
				ProposerIndex: 0,
				Body:          &types.BlockBody{},
			},
			ProposerAttestation: &types.Attestation{
				ValidatorID: 0,
				Data: &types.AttestationData{
					Slot:   1,
					Head:   &types.Checkpoint{},
					Target: &types.Checkpoint{},
					Source: &types.Checkpoint{},
				},
			},
		},
		Signature: &types.BlockSignatures{
			ProposerSignature: sig,
		},
	}

	// SSZ marshal (simulates P2P send)
	encoded, err := signedBlock.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// SSZ unmarshal (simulates P2P receive)
	decoded := &types.SignedBlockWithAttestation{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Extract ProposerSignature from decoded block (this is what processProposerAttestation does)
	extractedSig := decoded.Signature.ProposerSignature

	// Verify the raw bytes match
	if extractedSig != sig {
		t.Fatal("signature bytes changed through block SSZ round-trip")
	}

	// Parse to C handle (what processProposerAttestation does)
	csig, err := ParseSignature(extractedSig[:])
	if err != nil {
		t.Fatalf("parse sig: %v", err)
	}
	defer FreeSignature(csig)

	cpk, err := ParsePublicKey(pkBytes)
	if err != nil {
		t.Fatalf("parse pk: %v", err)
	}
	defer FreePublicKey(cpk)

	// Aggregate with slot=1 and the attestation data root as message
	EnsureProverReady()
	proof, err := AggregateSignatures([]CPubKey{cpk}, []CSig{csig}, attDataRoot, 1)
	if err != nil {
		t.Fatalf("aggregate FAILED: %v", err)
	}
	t.Logf("aggregate succeeded: proof=%d bytes", len(proof))

	// Verify
	EnsureVerifierReady()
	if err := VerifyAggregatedSignature(proof, []CPubKey{cpk}, attDataRoot, 1); err != nil {
		t.Fatalf("verify FAILED: %v", err)
	}
	t.Log("verify succeeded")
}
