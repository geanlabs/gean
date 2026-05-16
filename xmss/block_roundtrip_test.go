package xmss

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// TestProposerSigThroughBlockSSZ simulates the exact P2P path:
// Node1 signs → builds SignedBlock → SSZ marshal → SSZ unmarshal →
// extract ProposerSignature → ParseSignature → aggregate.
func TestProposerSigThroughBlockSSZ(t *testing.T) {
	kp, err := GenerateKeyPair("block-roundtrip-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	pkBytes, _ := kp.PublicKeyBytes()

	// Build a block and sign its root
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		Body:          &types.BlockBody{},
	}
	blockRoot, _ := block.HashTreeRoot()

	sig, err := kp.Sign(1, blockRoot) // slot 1
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Build a SignedBlock with this signature as ProposerSignature
	signedBlock := &types.SignedBlock{
		Block: block,
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
	decoded := &types.SignedBlock{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Extract ProposerSignature from decoded block
	extractedSig := decoded.Signature.ProposerSignature

	// Verify the raw bytes match
	if extractedSig != sig {
		t.Fatal("signature bytes changed through block SSZ round-trip")
	}

	// Parse to C handle
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

	// Aggregate with slot=1 and the block root as message
	EnsureProverReady()
	proof, err := AggregateSignatures([]CPubKey{cpk}, []CSig{csig}, blockRoot, 1)
	if err != nil {
		t.Fatalf("aggregate FAILED: %v", err)
	}
	t.Logf("aggregate succeeded: proof=%d bytes", len(proof))

	// Verify
	EnsureVerifierReady()
	if err := VerifyAggregatedSignature(proof, []CPubKey{cpk}, blockRoot, 1); err != nil {
		t.Fatalf("verify FAILED: %v", err)
	}
	t.Log("verify succeeded")
}
