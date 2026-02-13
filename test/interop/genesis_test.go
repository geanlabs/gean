package interop

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/devylongs/gean/chain/statetransition"
	"github.com/devylongs/gean/types"
)

// Reference roots generated from leanSpec at commit 4b750f2:
//   from lean_spec.subspecs.ssz.hash import hash_tree_root
//   from lean_spec.subspecs.containers.state.state import State
//   s = State.generate_genesis(Uint64(T), Uint64(N))
//   hash_tree_root(s).hex()

func TestGenesisStateRootMatchesLeanSpec(t *testing.T) {
	tests := []struct {
		genesisTime   uint64
		numValidators uint64
		expectedRoot  string
	}{
		{1000, 5, "8b819665c0de49890e492af3609e9b7704a3f1ca63cc2741747a4e5368c7a1ca"},
		{0, 5, "772c7ec9e8f2327f92922451ffc6c6781cb4673347162a30511de75a6e4e4817"},
		{1000, 3, "d63c807b2a32e003e61b7df76b9996d004e47979d822adfb5f450cfbea95b7be"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("time=%d_n=%d", tt.genesisTime, tt.numValidators), func(t *testing.T) {
			state := statetransition.GenerateGenesis(tt.genesisTime, tt.numValidators)
			root, err := state.HashTreeRoot()
			if err != nil {
				t.Fatalf("HashTreeRoot: %v", err)
			}
			got := hex.EncodeToString(root[:])
			if got != tt.expectedRoot {
				t.Errorf("genesis root mismatch:\n  got:  %s\n  want: %s", got, tt.expectedRoot)
				debugGenesisFields(t, state)
			}
		})
	}
}

func TestEmptyBlockBodyRoot(t *testing.T) {
	// Reference from leanSpec: hash_tree_root(BlockBody(attestations=Attestations(data=[])))
	expected := "dba9671bac9513c9482f1416a53aabd2c6ce90d5a5f865ce5a55c775325c9136"

	body := &types.BlockBody{Attestations: []*types.SignedVote{}}
	root, err := body.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	got := hex.EncodeToString(root[:])
	if got != expected {
		t.Errorf("empty body root mismatch:\n  got:  %s\n  want: %s", got, expected)
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
	expected := "ed01b1825c7b112c8b9c6e0f41c4d49e400fc120425582e533c332a6ac46082e"

	body := &types.BlockBody{Attestations: []*types.SignedVote{}}
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
	got := hex.EncodeToString(root[:])
	if got != expected {
		t.Errorf("genesis header root mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestConfigRoot(t *testing.T) {
	expected := "8ef40f45cfdd5684d5bfa333c650f233cb05edab4183f2191baeb91ed4fae9dd"

	cfg := &types.Config{NumValidators: 5, GenesisTime: 1000}
	root, err := cfg.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	got := hex.EncodeToString(root[:])
	if got != expected {
		t.Errorf("config root mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
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
	t.Logf("  justif roots len: %d", len(state.JustificationsRoots))
	t.Logf("  justif vals bits: %x", state.JustificationsValidators)
}
