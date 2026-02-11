package consensus

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// helper to create a genesis state and first block for testing
func setupGenesisAndFirstBlock(t *testing.T) (*types.State, *types.Block) {
	t.Helper()
	state, genesisBlock := GenerateGenesis(1000000000, makeTestValidators(8))

	// Advance state to slot 1
	advanced, err := ProcessSlots(state, 1)
	if err != nil {
		t.Fatalf("process slots: %v", err)
	}

	genesisHeaderRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1, // slot 1 % 8 = 1
		ParentRoot:    genesisHeaderRoot,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: []types.Attestation{}},
	}

	postState, err := ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("process block: %v", err)
	}

	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	_ = genesisBlock // suppress unused
	return postState, block
}

func TestProcessSlots_FillsStateRootWhenAdvancing(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))

	// Genesis header has zero state root
	if !state.LatestBlockHeader.StateRoot.IsZero() {
		t.Fatal("expected zero state root in genesis header")
	}

	processed, err := ProcessSlots(state, 1)
	if err != nil {
		t.Fatalf("process slots: %v", err)
	}

	if processed.LatestBlockHeader.StateRoot.IsZero() {
		t.Error("state root should be filled during ProcessSlots")
	}
}

func TestProcessSlots_DoesNotRewriteFilledStateRoot(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))

	// First ProcessSlots call fills state root for genesis header.
	filled, err := ProcessSlots(state, 1)
	if err != nil {
		t.Fatalf("process slots to 1: %v", err)
	}
	rootBefore := filled.LatestBlockHeader.StateRoot

	// Next ProcessSlots call should not rewrite the already-filled header state root.
	reprocessed, err := ProcessSlots(filled, 2)
	if err != nil {
		t.Fatalf("process slots to 2: %v", err)
	}

	if reprocessed.LatestBlockHeader.StateRoot != rootBefore {
		t.Error("ProcessSlots should preserve already-filled header state root")
	}
}

func TestProcessSlots_AdvancesCorrectly(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))

	advanced, err := ProcessSlots(state, 5)
	if err != nil {
		t.Fatalf("process slots: %v", err)
	}

	if advanced.Slot != 5 {
		t.Errorf("slot = %d, want 5", advanced.Slot)
	}
}

func TestProcessSlots_ErrorIfNotFuture(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))

	_, err := ProcessSlots(state, 0)
	if err == nil {
		t.Error("expected error for non-future target slot")
	}
}

func TestProcessBlockHeader_Valid(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	advanced, _ := ProcessSlots(state, 1)

	headerRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1, // 1 % 8 = 1
		ParentRoot:    headerRoot,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{},
	}

	result, err := ProcessBlockHeader(advanced, block)
	if err != nil {
		t.Fatalf("ProcessBlockHeader: %v", err)
	}

	if result.LatestBlockHeader.Slot != 1 {
		t.Errorf("header slot = %d, want 1", result.LatestBlockHeader.Slot)
	}
}

func TestProcessBlockHeader_WrongSlot(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	advanced, _ := ProcessSlots(state, 1)

	block := &types.Block{
		Slot: 2, // wrong — state is at slot 1
	}

	_, err := ProcessBlockHeader(advanced, block)
	if err == nil {
		t.Error("expected error for slot mismatch")
	}
}

func TestProcessBlockHeader_WrongProposer(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	advanced, _ := ProcessSlots(state, 1)

	headerRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0, // wrong — slot 1 % 8 = 1
		ParentRoot:    headerRoot,
	}

	_, err := ProcessBlockHeader(advanced, block)
	if err == nil {
		t.Error("expected error for wrong proposer")
	}
}

func TestProcessBlockHeader_WrongParent(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	advanced, _ := ProcessSlots(state, 1)

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    types.Root{0xff}, // wrong parent
	}

	_, err := ProcessBlockHeader(advanced, block)
	if err == nil {
		t.Error("expected error for wrong parent root")
	}
}

func TestProcessBlockHeader_GenesisSpecialCase(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	advanced, _ := ProcessSlots(state, 1)

	headerRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    headerRoot,
		Body:          types.BlockBody{},
	}

	result, err := ProcessBlockHeader(advanced, block)
	if err != nil {
		t.Fatalf("ProcessBlockHeader: %v", err)
	}

	// First block after genesis should set justified/finalized root to parent root
	if result.LatestJustified.Root != headerRoot {
		t.Error("latest justified root should be set to genesis header root")
	}
	if result.LatestFinalized.Root != headerRoot {
		t.Error("latest finalized root should be set to genesis header root")
	}
}

func TestProcessBlockHeader_EmptySlots(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))
	// Advance to slot 3 (skipping slot 1 and 2)
	advanced, _ := ProcessSlots(state, 3)

	headerRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          3,
		ProposerIndex: 3, // 3 % 8 = 3
		ParentRoot:    headerRoot,
		Body:          types.BlockBody{},
	}

	result, err := ProcessBlockHeader(advanced, block)
	if err != nil {
		t.Fatalf("ProcessBlockHeader: %v", err)
	}

	// Should have parent hash + 2 zero hashes for skipped slots (1, 2)
	if len(result.HistoricalBlockHashes) != 3 {
		t.Errorf("historical hashes = %d, want 3", len(result.HistoricalBlockHashes))
	}

	// First entry is parent root, rest are zero
	if result.HistoricalBlockHashes[0] != headerRoot {
		t.Error("first historical hash should be parent root")
	}
	if !result.HistoricalBlockHashes[1].IsZero() {
		t.Error("second hash should be zero (skipped slot)")
	}
	if !result.HistoricalBlockHashes[2].IsZero() {
		t.Error("third hash should be zero (skipped slot)")
	}
}

// makeTestState creates a minimal state for ProcessAttestations testing.
func makeTestState(numValidators uint64, slot types.Slot, justified, finalized types.Checkpoint) *types.State {
	return &types.State{
		Config:                  types.Config{GenesisTime: 1000},
		Slot:                    slot,
		JustifiedSlots:          bitfield.NewBitlist(4),
		Validators:              makeTestValidators(numValidators),
		LatestJustified:         justified,
		LatestFinalized:         finalized,
		JustificationRoots:      []types.Root{},
		JustificationValidators: bitfield.NewBitlist(0),
		HistoricalBlockHashes:   []types.Root{},
	}
}

func TestProcessAttestations_JustifiesTarget(t *testing.T) {
	// Create a state where slot 0 is justified
	state := makeTestState(4, 3,
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
	)
	// Mark slot 0 as justified
	bitfield.Bitlist(state.JustifiedSlots).SetBitAt(0, true)

	attestations := []types.Attestation{
		{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   2,
				Source: types.Checkpoint{Root: types.Root{1}, Slot: 0},
				Target: types.Checkpoint{Root: types.Root{2}, Slot: 2},
			},
		},
	}

	result, err := ProcessAttestations(state, attestations)
	if err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	// Target slot 2 should now be justified
	bl := bitfield.Bitlist(result.JustifiedSlots)
	if bl.Len() <= 2 || !bl.BitAt(2) {
		t.Error("target slot 2 should be justified")
	}

	if result.LatestJustified.Slot != 2 {
		t.Errorf("latest justified slot = %d, want 2", result.LatestJustified.Slot)
	}
}

func TestProcessAttestations_FinalizesSource(t *testing.T) {
	// State with slots 0 and 1 justified (consecutive)
	state := makeTestState(4, 5,
		types.Checkpoint{Root: types.Root{2}, Slot: 1},
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
	)
	bl := bitfield.Bitlist(state.JustifiedSlots)
	bl.SetBitAt(0, true)
	bl.SetBitAt(1, true)

	// Vote with source=0 target=1 (both already justified, consecutive)
	attestations := []types.Attestation{
		{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   3,
				Source: types.Checkpoint{Root: types.Root{1}, Slot: 0},
				Target: types.Checkpoint{Root: types.Root{2}, Slot: 1},
			},
		},
	}

	result, err := ProcessAttestations(state, attestations)
	if err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	// Source (slot 0) should be finalized, target (slot 1) should be latest justified
	if result.LatestFinalized.Slot != 0 {
		t.Errorf("latest finalized slot = %d, want 0", result.LatestFinalized.Slot)
	}
	if result.LatestJustified.Slot != 1 {
		t.Errorf("latest justified slot = %d, want 1", result.LatestJustified.Slot)
	}
}

func TestProcessAttestations_FinalizesAcrossGap(t *testing.T) {
	// Finalized=0, slots 0, 6, 9 justified.
	// Slots 7 and 8 are NOT justifiable after finalized=0 (delta 7,8: not <=5, not square, not pronic).
	// So source=6, target=9 should finalize source=6 (no justifiable gap between 7..8).
	state := makeTestState(4, 15,
		types.Checkpoint{Root: types.Root{9}, Slot: 9},
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
	)
	state.JustifiedSlots = bitfield.NewBitlist(10)
	bl := bitfield.Bitlist(state.JustifiedSlots)
	bl.SetBitAt(0, true)
	bl.SetBitAt(6, true)
	bl.SetBitAt(9, true)

	attestations := []types.Attestation{
		{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   10,
				Source: types.Checkpoint{Root: types.Root{6}, Slot: 6},
				Target: types.Checkpoint{Root: types.Root{9}, Slot: 9},
			},
		},
	}

	result, err := ProcessAttestations(state, attestations)
	if err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	if result.LatestFinalized.Slot != 6 {
		t.Errorf("latest finalized slot = %d, want 6", result.LatestFinalized.Slot)
	}
}

func TestProcessAttestations_NoFinalizeWithGap(t *testing.T) {
	// Finalized=0, slots 0 and 4 justified.
	// Slots 1,2,3 ARE justifiable after finalized=0 (delta <=5).
	// So source=0, target=4 should NOT finalize (justifiable gap exists).
	state := makeTestState(4, 10,
		types.Checkpoint{Root: types.Root{4}, Slot: 4},
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
	)
	state.JustifiedSlots = bitfield.NewBitlist(5)
	bl := bitfield.Bitlist(state.JustifiedSlots)
	bl.SetBitAt(0, true)
	bl.SetBitAt(4, true)

	attestations := []types.Attestation{
		{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   5,
				Source: types.Checkpoint{Root: types.Root{1}, Slot: 0},
				Target: types.Checkpoint{Root: types.Root{4}, Slot: 4},
			},
		},
	}

	result, err := ProcessAttestations(state, attestations)
	if err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	// Should NOT finalize because slots 1,2,3 are justifiable after finalized=0
	if result.LatestFinalized.Slot != 0 {
		t.Errorf("latest finalized slot = %d, want 0 (gap exists)", result.LatestFinalized.Slot)
	}
}

func TestProcessAttestations_SkipsInvalid(t *testing.T) {
	state := makeTestState(4, 5,
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
		types.Checkpoint{Root: types.Root{1}, Slot: 0},
	)
	bitfield.Bitlist(state.JustifiedSlots).SetBitAt(0, true)

	// Invalid: source.Slot >= target.Slot
	attestations := []types.Attestation{
		{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   3,
				Source: types.Checkpoint{Root: types.Root{1}, Slot: 2},
				Target: types.Checkpoint{Root: types.Root{2}, Slot: 1},
			},
		},
	}

	result, err := ProcessAttestations(state, attestations)
	if err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	// Nothing should change
	if result.LatestJustified.Slot != 0 {
		t.Errorf("latest justified should not change, got slot %d", result.LatestJustified.Slot)
	}
}

func TestProcessBlock_EndToEnd(t *testing.T) {
	postState, block := setupGenesisAndFirstBlock(t)

	if postState.LatestBlockHeader.Slot != 1 {
		t.Errorf("post-state header slot = %d, want 1", postState.LatestBlockHeader.Slot)
	}
	if block.Slot != 1 {
		t.Errorf("block slot = %d, want 1", block.Slot)
	}
}

func TestCopy_DeepIndependence(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, makeTestValidators(8))

	cp := Copy(state)
	cp.Slot = 99
	cp.HistoricalBlockHashes = append(cp.HistoricalBlockHashes, types.Root{1})

	if state.Slot == 99 {
		t.Error("modifying copy should not affect original slot")
	}
	if len(state.HistoricalBlockHashes) != 0 {
		t.Error("modifying copy should not affect original historical hashes")
	}
}
