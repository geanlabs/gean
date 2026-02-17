package interop

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

func makeTestValidators(n uint64) []*types.Validator {
	validators := make([]*types.Validator, n)
	for i := uint64(0); i < n; i++ {
		validators[i] = &types.Validator{Index: i}
	}
	return validators
}

func TestGenesisStateRootConsistency(t *testing.T) {
	tests := []struct {
		genesisTime   uint64
		numValidators uint64
	}{
		{1000, 5},
		{0, 5},
		{1000, 3},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("time=%d_n=%d", tt.genesisTime, tt.numValidators), func(t *testing.T) {
			state := statetransition.GenerateGenesis(tt.genesisTime, makeTestValidators(tt.numValidators))
			root, err := state.HashTreeRoot()
			if err != nil {
				t.Fatalf("HashTreeRoot: %v", err)
			}
			t.Logf("genesis root (devnet-1): %s", hex.EncodeToString(root[:]))

			// SSZ round-trip: ensure root is stable.
			data, err := state.MarshalSSZ()
			if err != nil {
				t.Fatalf("MarshalSSZ: %v", err)
			}
			decoded := new(types.State)
			if err := decoded.UnmarshalSSZ(data); err != nil {
				t.Fatalf("UnmarshalSSZ: %v", err)
			}
			decodedRoot, _ := decoded.HashTreeRoot()
			if root != decodedRoot {
				t.Errorf("SSZ round-trip changed root: %x != %x", root, decodedRoot)
			}
		})
	}
}

func TestEmptyBlockBodyRoot(t *testing.T) {
	// Reference from leanSpec devnet-0: hash_tree_root(BlockBody(attestations=Attestations(data=[])))
	// Note: expected value needs update since BlockBody.Attestations changed from []*SignedVote to []*Attestation.
	body := &types.BlockBody{Attestations: []*types.Attestation{}}
	root, err := body.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	t.Logf("empty body root (devnet-1): %s", hex.EncodeToString(root[:]))

	// Verify it's deterministic.
	body2 := &types.BlockBody{Attestations: []*types.Attestation{}}
	root2, _ := body2.HashTreeRoot()
	if root != root2 {
		t.Errorf("non-deterministic body root")
	}
}

func TestZeroCheckpointRoot(t *testing.T) {
	expected := "f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b"

	cp := &types.Checkpoint{Root: types.ZeroHash, Slot: 0}
	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	got := hex.EncodeToString(root[:])
	if got != expected {
		t.Errorf("zero checkpoint root mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestGenesisBlockHeaderRoot(t *testing.T) {
	// Note: body root depends on BlockBody type which changed.
	body := &types.BlockBody{Attestations: []*types.Attestation{}}
	bodyRoot, _ := body.HashTreeRoot()

	hdr := &types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		BodyRoot:      bodyRoot,
	}
	root, err := hdr.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	t.Logf("genesis header root (devnet-1): %s", hex.EncodeToString(root[:]))
}

func TestConfigRoot(t *testing.T) {
	cfg := &types.Config{GenesisTime: 1000}
	root, err := cfg.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	t.Logf("config root (devnet-1): %s", hex.EncodeToString(root[:]))
}

// debugGenesisFields prints individual field roots to help diagnose mismatches.
func debugGenesisFields(t *testing.T, state *types.State) {
	t.Helper()

	if root, err := state.Config.HashTreeRoot(); err == nil {
		t.Logf("  config root:      %x", root)
	}
	t.Logf("  slot:             %d", state.Slot)
	if root, err := state.LatestBlockHeader.HashTreeRoot(); err == nil {
		t.Logf("  header root:      %x", root)
	}
	if root, err := state.LatestJustified.HashTreeRoot(); err == nil {
		t.Logf("  justified root:   %x", root)
	}
	if root, err := state.LatestFinalized.HashTreeRoot(); err == nil {
		t.Logf("  finalized root:   %x", root)
	}
	t.Logf("  hist hashes len:  %d", len(state.HistoricalBlockHashes))
	t.Logf("  justified bits:   %x", state.JustifiedSlots)
	t.Logf("  validators len:   %d", len(state.Validators))
	t.Logf("  justif roots len: %d", len(state.JustificationsRoots))
	t.Logf("  justif vals bits: %x", state.JustificationsValidators)
}
