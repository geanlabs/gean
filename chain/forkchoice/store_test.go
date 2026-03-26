package forkchoice

import (
	"encoding/binary"
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

func TestPersistRestoreMetadata_StoresCheckpointAnchor(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	store := memory.New()
	fc := NewStoreFromCheckpointState(state, anchorRoot, store)

	if err := fc.PersistRestoreMetadata(); err != nil {
		t.Fatalf("PersistRestoreMetadata returned error: %v", err)
	}

	headData, ok := store.GetMeta(metaHeadKey)
	if !ok {
		t.Fatal("expected head metadata")
	}
	if got, err := decodeRoot(headData); err != nil {
		t.Fatalf("decode head metadata: %v", err)
	} else if got != anchorRoot {
		t.Fatalf("metadata head root = %x, want %x", got, anchorRoot)
	}

	anchorData, ok := store.GetMeta(metaCheckpointAnchorKey)
	if !ok {
		t.Fatal("expected checkpoint anchor metadata")
	}
	if got, err := decodeRoot(anchorData); err != nil {
		t.Fatalf("decode checkpoint anchor metadata: %v", err)
	} else if got != anchorRoot {
		t.Fatalf("metadata checkpoint anchor = %x, want %x", got, anchorRoot)
	}
}

func TestRestoreFromDB_UsesPersistedCanonicalMetadata(t *testing.T) {
	store := memory.New()

	canonicalRoot := [32]byte{0xA1}
	nonCanonicalRoot := [32]byte{0xB2}

	canonicalBlock := makeStoredBlock(10, [32]byte{0x01})
	nonCanonicalBlock := makeStoredBlock(11, [32]byte{0x02})
	store.PutBlock(canonicalRoot, canonicalBlock)
	store.PutSignedBlock(canonicalRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: canonicalBlock},
	})
	store.PutState(canonicalRoot, makeStoredState(10, canonicalBlock.ParentRoot, &types.Checkpoint{Root: canonicalRoot, Slot: 10}, &types.Checkpoint{Root: canonicalRoot, Slot: 10}, [][32]byte{{0x10}, canonicalRoot}))

	store.PutBlock(nonCanonicalRoot, nonCanonicalBlock)
	store.PutSignedBlock(nonCanonicalRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: nonCanonicalBlock},
	})
	store.PutState(nonCanonicalRoot, makeStoredState(11, nonCanonicalBlock.ParentRoot, &types.Checkpoint{Root: nonCanonicalRoot, Slot: 11}, &types.Checkpoint{Root: nonCanonicalRoot, Slot: 11}, [][32]byte{{0x20}, nonCanonicalRoot}))

	blockSummaries := map[[32]byte]blockSummary{
		canonicalRoot:    summarizeBlock(canonicalBlock),
		nonCanonicalRoot: summarizeBlock(nonCanonicalBlock),
	}
	fc := newRestoredStore(
		store,
		canonicalRoot,
		mustState(t, store, canonicalRoot),
		blockSummaries,
		buildCheckpointRootIndex(mustState(t, store, canonicalRoot), canonicalRoot),
		canonicalRoot,
		&types.Checkpoint{Root: canonicalRoot, Slot: 10},
		&types.Checkpoint{Root: canonicalRoot, Slot: 10},
		[32]byte{},
		false,
	)
	if err := fc.PersistRestoreMetadata(); err != nil {
		t.Fatalf("PersistRestoreMetadata returned error: %v", err)
	}

	restored := RestoreFromDB(store)
	if restored == nil {
		t.Fatal("expected store to restore")
	}
	status := restored.GetStatus()
	if status.Head != canonicalRoot {
		t.Fatalf("restored head = %x, want canonical root %x", status.Head, canonicalRoot)
	}
	if status.HeadSlot != 10 {
		t.Fatalf("restored head slot = %d, want 10", status.HeadSlot)
	}
}

func TestRestoreFromDB_FallsBackWhenMetadataInvalid(t *testing.T) {
	store := memory.New()

	canonicalRoot := [32]byte{0xC1}
	fallbackRoot := [32]byte{0xD2}

	canonicalBlock := makeStoredBlock(10, [32]byte{0x03})
	fallbackBlock := makeStoredBlock(11, [32]byte{0x04})
	store.PutBlock(canonicalRoot, canonicalBlock)
	store.PutSignedBlock(canonicalRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: canonicalBlock},
	})
	store.PutState(canonicalRoot, makeStoredState(10, canonicalBlock.ParentRoot, &types.Checkpoint{Root: canonicalRoot, Slot: 10}, &types.Checkpoint{Root: canonicalRoot, Slot: 10}, [][32]byte{{0x30}, canonicalRoot}))

	store.PutBlock(fallbackRoot, fallbackBlock)
	store.PutSignedBlock(fallbackRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: fallbackBlock},
	})
	store.PutState(fallbackRoot, makeStoredState(11, fallbackBlock.ParentRoot, &types.Checkpoint{Root: fallbackRoot, Slot: 11}, &types.Checkpoint{Root: fallbackRoot, Slot: 11}, [][32]byte{{0x40}, fallbackRoot}))

	fc := newRestoredStore(
		store,
		canonicalRoot,
		mustState(t, store, canonicalRoot),
		map[[32]byte]blockSummary{
			canonicalRoot: summarizeBlock(canonicalBlock),
			fallbackRoot:  summarizeBlock(fallbackBlock),
		},
		buildCheckpointRootIndex(mustState(t, store, canonicalRoot), canonicalRoot),
		canonicalRoot,
		&types.Checkpoint{Root: canonicalRoot, Slot: 10},
		&types.Checkpoint{Root: canonicalRoot, Slot: 10},
		[32]byte{},
		false,
	)
	if err := fc.PersistRestoreMetadata(); err != nil {
		t.Fatalf("PersistRestoreMetadata returned error: %v", err)
	}

	store.DeleteStates([][32]byte{canonicalRoot})

	restored := RestoreFromDB(store)
	if restored == nil {
		t.Fatal("expected fallback restore to succeed")
	}
	status := restored.GetStatus()
	if status.Head != fallbackRoot {
		t.Fatalf("restored head = %x, want fallback root %x", status.Head, fallbackRoot)
	}
	if status.HeadSlot != 11 {
		t.Fatalf("restored head slot = %d, want 11", status.HeadSlot)
	}
}

func TestPruneOldData_PreservesCheckpointAnchor(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	store := memory.New()
	fc := NewStoreFromCheckpointState(state, anchorRoot, store)

	roots := make([][32]byte, 0, 5)
	for slot := uint64(4); slot <= 8; slot++ {
		root := [32]byte{byte(slot)}
		roots = append(roots, root)
		block := makeStoredBlock(slot, anchorRoot)
		fc.blockSummaries[root] = summarizeBlock(block)
		store.PutBlock(root, block)
		store.PutSignedBlock(root, &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		})
		store.PutState(root, makeStoredState(slot, block.ParentRoot, &types.Checkpoint{Root: root, Slot: slot}, &types.Checkpoint{Root: root, Slot: slot}, [][32]byte{anchorRoot, root}))
	}

	fc.head = roots[len(roots)-1]
	fc.safeTarget = roots[len(roots)-2]
	fc.latestJustified = &types.Checkpoint{Root: roots[len(roots)-2], Slot: 7}
	fc.latestFinalized = &types.Checkpoint{Root: roots[len(roots)-3], Slot: 6}

	prunedBlocks, prunedStates := fc.pruneOldDataLocked(2, 2)
	if prunedBlocks == 0 || prunedStates == 0 {
		t.Fatalf("expected pruning to remove older roots, got blocks=%d states=%d", prunedBlocks, prunedStates)
	}
	if _, ok := store.GetBlock(anchorRoot); !ok {
		t.Fatal("expected checkpoint anchor block to remain after pruning")
	}
	if _, ok := store.GetState(anchorRoot); !ok {
		t.Fatal("expected checkpoint anchor state to remain after pruning")
	}
}

func TestRestartAfterPrune_RestoresCorrectHeadAndCheckpoints(t *testing.T) {
	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	store := memory.New()
	fc := NewStoreFromCheckpointState(state, anchorRoot, store)

	roots := make([][32]byte, 0, 6)
	for slot := uint64(4); slot <= 9; slot++ {
		root := [32]byte{byte(slot + 0x20)}
		roots = append(roots, root)
		parent := anchorRoot
		if len(roots) > 1 {
			parent = roots[len(roots)-2]
		}
		block := makeStoredBlock(slot, parent)
		fc.blockSummaries[root] = summarizeBlock(block)
		store.PutBlock(root, block)
		store.PutSignedBlock(root, &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		})
		store.PutState(root, makeStoredState(slot, parent, &types.Checkpoint{Root: roots[2], Slot: 6}, &types.Checkpoint{Root: roots[1], Slot: 5}, append([][32]byte{anchorRoot}, roots...)))
	}

	fc.head = roots[len(roots)-1]
	fc.safeTarget = roots[len(roots)-2]
	fc.latestJustified = &types.Checkpoint{Root: roots[2], Slot: 6}
	fc.latestFinalized = &types.Checkpoint{Root: roots[1], Slot: 5}

	if err := fc.PersistRestoreMetadata(); err != nil {
		t.Fatalf("PersistRestoreMetadata returned error: %v", err)
	}
	fc.pruneOldDataLocked(2, 2)

	restored := RestoreFromDB(store)
	if restored == nil {
		t.Fatal("expected restore after prune to succeed")
	}
	status := restored.GetStatus()
	if status.Head != fc.head {
		t.Fatalf("restored head = %x, want %x", status.Head, fc.head)
	}
	if status.JustifiedRoot != fc.latestJustified.Root || status.JustifiedSlot != fc.latestJustified.Slot {
		t.Fatalf("restored justified = (%x,%d), want (%x,%d)", status.JustifiedRoot, status.JustifiedSlot, fc.latestJustified.Root, fc.latestJustified.Slot)
	}
	if status.FinalizedRoot != fc.latestFinalized.Root || status.FinalizedSlot != fc.latestFinalized.Slot {
		t.Fatalf("restored finalized = (%x,%d), want (%x,%d)", status.FinalizedRoot, status.FinalizedSlot, fc.latestFinalized.Root, fc.latestFinalized.Slot)
	}
	if !restored.hasCheckpointAnchor || restored.checkpointAnchorRoot != anchorRoot {
		t.Fatalf("restored checkpoint anchor = (%t,%x), want (%t,%x)", restored.hasCheckpointAnchor, restored.checkpointAnchorRoot, true, anchorRoot)
	}
}

func TestDecodeCheckpoint_RoundTrip(t *testing.T) {
	cp := &types.Checkpoint{Root: [32]byte{0xAB}, Slot: 42}
	encoded, err := encodeCheckpoint(cp)
	if err != nil {
		t.Fatalf("encodeCheckpoint returned error: %v", err)
	}
	decoded, err := decodeCheckpoint(encoded)
	if err != nil {
		t.Fatalf("decodeCheckpoint returned error: %v", err)
	}
	if decoded.Root != cp.Root || decoded.Slot != cp.Slot {
		t.Fatalf("decoded checkpoint = (%x,%d), want (%x,%d)", decoded.Root, decoded.Slot, cp.Root, cp.Slot)
	}
	if binary.LittleEndian.Uint64(encoded[32:]) != cp.Slot {
		t.Fatalf("expected little-endian slot encoding, got %d", binary.LittleEndian.Uint64(encoded[32:]))
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

func makeStoredBlock(slot uint64, parentRoot [32]byte) *types.Block {
	return &types.Block{
		Slot:          slot,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.AggregatedAttestation{}},
	}
}

func makeStoredState(slot uint64, parentRoot [32]byte, justified *types.Checkpoint, finalized *types.Checkpoint, history [][32]byte) *types.State {
	return &types.State{
		Config: &types.Config{GenesisTime: 1234},
		Slot:   slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          slot,
			ProposerIndex: 0,
			ParentRoot:    parentRoot,
			StateRoot:     types.ZeroHash,
			BodyRoot:      types.ZeroHash,
		},
		LatestJustified:          cloneCheckpoint(justified),
		LatestFinalized:          cloneCheckpoint(finalized),
		HistoricalBlockHashes:    append([][32]byte(nil), history...),
		JustifiedSlots:           []byte{0x01},
		Validators:               []*types.Validator{{Index: 0, Pubkey: [52]byte{0x01}}},
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01},
	}
}

func mustState(t *testing.T, store *memory.Store, root [32]byte) *types.State {
	t.Helper()
	state, ok := store.GetState(root)
	if !ok {
		t.Fatalf("expected state for root %x", root)
	}
	return state
}
