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

	if summary, ok := fc.blockSummaries[anchorRoot]; !ok {
		t.Fatal("expected anchor root to be indexed in block summaries")
	} else if summary.Slot != state.Slot {
		t.Fatalf("anchor summary slot = %d, want %d", summary.Slot, state.Slot)
	}
}

func TestLookupBlockSummary_UsesRuntimeIndex(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	fc := NewStoreFromCheckpointState(state, anchorRoot, memory.New())

	childRoot := [32]byte{0x44}
	fc.blockSummaries[childRoot] = blockSummary{
		Slot:          4,
		ParentRoot:    anchorRoot,
		ProposerIndex: 1,
	}
	fc.storage = memory.New()

	summary, ok := fc.lookupBlockSummary(childRoot)
	if !ok {
		t.Fatal("expected lookupBlockSummary to resolve runtime summary without storage")
	}
	if summary.Slot != 4 {
		t.Fatalf("summary slot = %d, want 4", summary.Slot)
	}
	if summary.ParentRoot != anchorRoot {
		t.Fatalf("summary parent = %x, want %x", summary.ParentRoot, anchorRoot)
	}
}

func TestAllKnownBlockSummaries_MergesRuntimeAndCheckpointRoots(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	fc := NewStoreFromCheckpointState(state, anchorRoot, memory.New())

	childRoot := [32]byte{0x55}
	fc.blockSummaries[childRoot] = blockSummary{
		Slot:          4,
		ParentRoot:    anchorRoot,
		ProposerIndex: 1,
	}
	fc.storage = memory.New()

	summaries := fc.allKnownBlockSummaries()
	if _, ok := summaries[childRoot]; !ok {
		t.Fatal("expected runtime block summary to be included")
	}
	if _, ok := summaries[state.LatestJustified.Root]; !ok {
		t.Fatal("expected checkpoint-root summary to be included")
	}
}

func TestPruneOldData_PreservesProtectedRootsAndWindows(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	store := memory.New()
	fc := NewStoreFromCheckpointState(state, anchorRoot, store)

	fc.blockSummaries = make(map[[32]byte]blockSummary)
	roots := make([][32]byte, 0, 6)
	for slot := uint64(1); slot <= 6; slot++ {
		root := [32]byte{byte(slot)}
		roots = append(roots, root)
		fc.blockSummaries[root] = blockSummary{Slot: slot}
		store.PutBlock(root, &types.Block{Slot: slot, Body: &types.BlockBody{}})
		store.PutSignedBlock(root, &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{
				Block: &types.Block{Slot: slot, Body: &types.BlockBody{}},
			},
		})
		store.PutState(root, &types.State{
			Slot:                     slot,
			Config:                   &types.Config{GenesisTime: 1},
			LatestJustified:          &types.Checkpoint{},
			LatestFinalized:          &types.Checkpoint{},
			LatestBlockHeader:        &types.BlockHeader{},
			JustifiedSlots:           []byte{0x01},
			JustificationsValidators: []byte{0x01},
		})
	}

	fc.head = roots[5]
	fc.safeTarget = roots[4]
	fc.latestJustified = &types.Checkpoint{Root: roots[3], Slot: 4}
	fc.latestFinalized = &types.Checkpoint{Root: roots[0], Slot: 1}

	prunedBlocks, prunedStates := fc.pruneOldDataLocked(3, 2)

	if prunedBlocks != 2 {
		t.Fatalf("prunedBlocks = %d, want 2", prunedBlocks)
	}
	if prunedStates != 2 {
		t.Fatalf("prunedStates = %d, want 2", prunedStates)
	}

	for _, root := range [][32]byte{roots[5], roots[4], roots[3], roots[0]} {
		if _, ok := store.GetBlock(root); !ok {
			t.Fatalf("expected protected/retained block %x to remain", root)
		}
		if _, ok := fc.blockSummaries[root]; !ok {
			t.Fatalf("expected protected/retained summary %x to remain", root)
		}
	}

	for _, root := range [][32]byte{roots[2], roots[1]} {
		if _, ok := store.GetBlock(root); ok {
			t.Fatalf("expected old block %x to be pruned", root)
		}
		if _, ok := fc.blockSummaries[root]; ok {
			t.Fatalf("expected old block summary %x to be pruned", root)
		}
	}

	for _, root := range [][32]byte{roots[2], roots[1]} {
		if _, ok := store.GetState(root); ok {
			t.Fatalf("expected old state %x to be pruned", root)
		}
	}
	if _, ok := store.GetState(roots[0]); !ok {
		t.Fatalf("expected protected finalized state %x to remain", roots[0])
	}
	for _, root := range [][32]byte{roots[5], roots[4], roots[3]} {
		if _, ok := store.GetState(root); !ok {
			t.Fatalf("expected retained state %x to remain", root)
		}
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
