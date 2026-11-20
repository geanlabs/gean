//go:build !fakecrypto

package importer

import (
	"testing"

	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestProposerSigThroughBlockSSZ(t *testing.T) {
	kp, err := xmss.GenerateKeyPair("block-roundtrip-0", 0, 1<<10)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		Body:          &types.BlockBody{},
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("block root: %v", err)
	}

	sig, err := kp.Sign(1, blockRoot)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	csig, err := xmss.ParseSignature(sig[:])
	if err != nil {
		t.Fatalf("parse sig: %v", err)
	}
	defer xmss.FreeSignature(csig)

	cpk, err := xmss.ParsePublicKey(pkBytes)
	if err != nil {
		t.Fatalf("parse pk: %v", err)
	}
	defer xmss.FreePublicKey(cpk)

	// A block without attestations: its proof is the proposer's raw signature alone.
	merged, err := xmss.MergeType1Proofs(nil, []xmss.RawSignature{{Pubkey: cpk, Signature: csig, Message: blockRoot, Slot: 1}})
	if err != nil {
		t.Fatalf("merge FAILED: %v", err)
	}
	signedBlock := &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{Proof: merged},
	}
	encoded, err := signedBlock.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &types.SignedBlock{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := xmss.VerifyType2Proof(
		decoded.Proof.Proof,
		[][]xmss.CPubKey{{cpk}},
		[]xmss.MessageBinding{{Message: blockRoot, Slot: 1}},
	); err != nil {
		t.Fatalf("verify FAILED: %v", err)
	}
}
