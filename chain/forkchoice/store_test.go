package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func TestNewStoreFromCheckpointState(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	store := memory.New()

	fc := NewStoreFromCheckpointState(state, anchorRoot, store)

	if _, ok := store.GetBlock(state.LatestJustified.Root); ok {
		t.Fatal("expected no stored placeholder for justified checkpoint root")
	}
	if _, ok := store.GetBlock(state.LatestFinalized.Root); ok {
		t.Fatal("expected no stored placeholder for finalized checkpoint root")
	}

	if proposalHead := fc.GetProposalHead(state.Slot + 1); proposalHead != anchorRoot {
		t.Fatalf("proposal head = %x, want %x", proposalHead, anchorRoot)
	}

	target, err := fc.GetVoteTarget()
	if err != nil {
		t.Fatalf("GetVoteTarget returned error: %v", err)
	}
	if target.Root != state.LatestJustified.Root {
		t.Fatalf("vote target root = %x, want %x", target.Root, state.LatestJustified.Root)
	}

	valid := fc.validateAttestationData(&types.AttestationData{
		Slot:   3,
		Head:   &types.Checkpoint{Root: anchorRoot, Slot: 3},
		Source: &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot},
		Target: &types.Checkpoint{Root: anchorRoot, Slot: 3},
	})
	if valid != "" {
		t.Fatalf("validateAttestationData returned %q, want success", valid)
	}
}

func prepareCheckpointStateForStore(t *testing.T, state *types.State) [32]byte {
	t.Helper()

	originalStateRoot := state.LatestBlockHeader.StateRoot
	state.LatestBlockHeader.StateRoot = types.ZeroHash
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot returned error: %v", err)
	}
	state.LatestBlockHeader.StateRoot = originalStateRoot

	prepared := state.Copy()
	prepared.LatestBlockHeader.StateRoot = stateRoot
	anchorRoot, err := prepared.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot header returned error: %v", err)
	}
	*state = *prepared
	return anchorRoot
}

func makeCheckpointState() *types.State {
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	validators := []*types.Validator{
		{Index: 0, Pubkey: [52]byte{0x01}},
		{Index: 1, Pubkey: [52]byte{0x02}},
	}

	return &types.State{
		Config: &types.Config{GenesisTime: 1234},
		Slot:   3,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          3,
			ProposerIndex: 0,
			ParentRoot:    [32]byte{0x11},
			StateRoot:     types.ZeroHash,
			BodyRoot:      bodyRoot,
		},
		LatestJustified:          &types.Checkpoint{Root: [32]byte{0x11}, Slot: 2},
		LatestFinalized:          &types.Checkpoint{Root: [32]byte{0x22}, Slot: 1},
		HistoricalBlockHashes:    [][32]byte{{0x33}, {0x22}, {0x11}},
		JustifiedSlots:           []byte{0x01},
		Validators:               validators,
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01},
	}
}
