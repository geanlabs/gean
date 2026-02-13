package interop

import (
	"encoding/hex"
	"testing"

	"github.com/geanlabs/gean/types"
)

// Reference SSZ bytes and roots generated from leanSpec at commit 4b750f2.

func TestSignedBlockSSZCrossClient(t *testing.T) {
	// Block: slot=1, proposer=0, parent=0xab*32, state=0xcd*32, empty body
	// Signature: 0xef*32
	var parentRoot, stateRoot, sig [32]byte
	for i := range parentRoot {
		parentRoot[i] = 0xab
		stateRoot[i] = 0xcd
		sig[i] = 0xef
	}

	sb := &types.SignedBlock{
		Message: &types.Block{
			Slot:          1,
			ProposerIndex: 0,
			ParentRoot:    parentRoot,
			StateRoot:     stateRoot,
			Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
		},
		Signature: sig,
	}

	data, err := sb.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}

	expectedSSZ := "24000000efefefefefefefefefefefefefefefefefefefefefefefefefefefefefefefef01000000000000000000000000000000ababababababababababababababababababababababababababababababababcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd5400000004000000"
	gotSSZ := hex.EncodeToString(data)
	if gotSSZ != expectedSSZ {
		t.Errorf("SignedBlock SSZ mismatch:\n  got:  %s\n  want: %s", gotSSZ, expectedSSZ)
	}

	// Verify hash tree root matches leanSpec.
	expectedRoot := "96006110de282b2b8258b6c4df79b324511d30f96c8f40698ed033246f262cf2"
	root, err := sb.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	gotRoot := hex.EncodeToString(root[:])
	if gotRoot != expectedRoot {
		t.Errorf("SignedBlock root mismatch:\n  got:  %s\n  want: %s", gotRoot, expectedRoot)
	}

	// Verify Block hash tree root.
	expectedBlockRoot := "1318f2c155a2ef5515839b449dff382128a5b51df0724add8e5a1f8b5743dcd7"
	blockRoot, err := sb.Message.HashTreeRoot()
	if err != nil {
		t.Fatalf("Block HashTreeRoot: %v", err)
	}
	gotBlockRoot := hex.EncodeToString(blockRoot[:])
	if gotBlockRoot != expectedBlockRoot {
		t.Errorf("Block root mismatch:\n  got:  %s\n  want: %s", gotBlockRoot, expectedBlockRoot)
	}

	// Round-trip: decode and re-encode.
	decoded := new(types.SignedBlock)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	reencoded, err := decoded.MarshalSSZ()
	if err != nil {
		t.Fatalf("re-MarshalSSZ: %v", err)
	}
	if hex.EncodeToString(reencoded) != expectedSSZ {
		t.Error("SSZ round-trip produced different bytes")
	}
}

func TestSignedVoteSSZCrossClient(t *testing.T) {
	// Vote: validator=2, slot=5, head=(0x11*32, 3), target=(0x22*32, 4), source=(0x33*32, 1)
	// Signature: 0x44*32
	var headRoot, targetRoot, sourceRoot, sig [32]byte
	for i := 0; i < 32; i++ {
		headRoot[i] = 0x11
		targetRoot[i] = 0x22
		sourceRoot[i] = 0x33
		sig[i] = 0x44
	}

	sv := &types.SignedVote{
		Data: &types.Vote{
			ValidatorID: 2,
			Slot:        5,
			Head:        &types.Checkpoint{Root: headRoot, Slot: 3},
			Target:      &types.Checkpoint{Root: targetRoot, Slot: 4},
			Source:      &types.Checkpoint{Root: sourceRoot, Slot: 1},
		},
		Signature: sig,
	}

	data, err := sv.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}

	expectedSSZ := "020000000000000005000000000000001111111111111111111111111111111111111111111111111111111111111111030000000000000022222222222222222222222222222222222222222222222222222222222222220400000000000000333333333333333333333333333333333333333333333333333333333333333301000000000000004444444444444444444444444444444444444444444444444444444444444444"
	gotSSZ := hex.EncodeToString(data)
	if gotSSZ != expectedSSZ {
		t.Errorf("SignedVote SSZ mismatch:\n  got:  %s\n  want: %s", gotSSZ, expectedSSZ)
	}

	// Verify hash tree roots match leanSpec.
	expectedRoot := "c8e262d072a46a3aca14e806c0fceb673a4cc9b79ba3e856da919139152e6b03"
	root, err := sv.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	gotRoot := hex.EncodeToString(root[:])
	if gotRoot != expectedRoot {
		t.Errorf("SignedVote root mismatch:\n  got:  %s\n  want: %s", gotRoot, expectedRoot)
	}

	expectedVoteRoot := "3564a38491708d78089d93e9bfc6aafb886d01591e0760bc511fa67ac69f5cc5"
	voteRoot, err := sv.Data.HashTreeRoot()
	if err != nil {
		t.Fatalf("Vote HashTreeRoot: %v", err)
	}
	gotVoteRoot := hex.EncodeToString(voteRoot[:])
	if gotVoteRoot != expectedVoteRoot {
		t.Errorf("Vote root mismatch:\n  got:  %s\n  want: %s", gotVoteRoot, expectedVoteRoot)
	}

	// Round-trip.
	decoded := new(types.SignedVote)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	reencoded, err := decoded.MarshalSSZ()
	if err != nil {
		t.Fatalf("re-MarshalSSZ: %v", err)
	}
	if hex.EncodeToString(reencoded) != expectedSSZ {
		t.Error("SSZ round-trip produced different bytes")
	}
}
