package statetransition

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func makeGenesisState(n int) *types.State {
	validators := make([]*types.Validator, n)
	for i := range n {
		var pubkey [types.PubkeySize]byte
		pubkey[0] = byte(i + 1)
		validators[i] = &types.Validator{AttestationPubkey: pubkey, ProposalPubkey: pubkey, Index: uint64(i)}
	}
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		HistoricalBlockHashes:    nil,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		Validators:               validators,
		JustificationsRoots:      nil,
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestProcessSlotsAdvancesSlot(t *testing.T) {
	state := makeGenesisState(3)
	if err := ProcessSlots(state, 5); err != nil {
		t.Fatal(err)
	}
	if state.Slot != 5 {
		t.Fatalf("expected slot 5, got %d", state.Slot)
	}
}

func TestProcessSlotsCachesStateRoot(t *testing.T) {
	state := makeGenesisState(3)
	if state.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatal("genesis header should have zero state root")
	}
	if err := ProcessSlots(state, 1); err != nil {
		t.Fatal(err)
	}
	if state.LatestBlockHeader.StateRoot == types.ZeroRoot {
		t.Fatal("state root should be cached after process_slots")
	}
}

func TestProcessSlotsRejectsOlderSlot(t *testing.T) {
	state := makeGenesisState(3)
	state.Slot = 10
	err := ProcessSlots(state, 5)
	if err == nil {
		t.Fatal("should reject target slot < current slot")
	}
	if _, ok := err.(*StateSlotIsNewerError); !ok {
		t.Fatalf("expected StateSlotIsNewerError, got %T", err)
	}
}

func TestProcessSlotsRejectsMalformedState(t *testing.T) {
	if err := ProcessSlots(nil, 1); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("ProcessSlots(nil) error=%v, want ErrMalformedState", err)
	}

	state := makeGenesisState(1)
	state.LatestBlockHeader = nil
	if err := ProcessSlots(state, 1); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("ProcessSlots without header error=%v, want ErrMalformedState", err)
	}
}

func TestProcessBlockHeaderValidatesSlot(t *testing.T) {
	state := makeGenesisState(3)
	state.Slot = 1

	block := &types.Block{
		Slot:          2,
		ProposerIndex: 0,
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject slot mismatch")
	}
}

func TestProcessBlockHeaderRejectsMalformedInputs(t *testing.T) {
	if err := ProcessBlockHeader(nil, &types.Block{}); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil state error=%v, want ErrMalformedState", err)
	}

	state := makeGenesisState(1)
	state.LatestBlockHeader = nil
	if err := ProcessBlockHeader(state, &types.Block{}); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil latest header error=%v, want ErrMalformedState", err)
	}

	state = makeGenesisState(1)
	state.LatestJustified = nil
	if err := ProcessBlockHeader(state, &types.Block{}); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil latest justified error=%v, want ErrMalformedState", err)
	}

	state = makeGenesisState(1)
	state.LatestFinalized = nil
	if err := ProcessBlockHeader(state, &types.Block{}); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil latest finalized error=%v, want ErrMalformedState", err)
	}

	state = makeGenesisState(1)
	if err := ProcessBlockHeader(state, nil); !errors.Is(err, ErrMalformedBlock) {
		t.Fatalf("nil block error=%v, want ErrMalformedBlock", err)
	}

	if err := ProcessBlockHeader(state, &types.Block{}); !errors.Is(err, ErrMalformedBlock) {
		t.Fatalf("nil block body error=%v, want ErrMalformedBlock", err)
	}

	state = makeGenesisState(1)
	ProcessSlots(state, 1)
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{Attestations: []*types.AggregatedAttestation{nil}},
	}
	if err := ProcessBlockHeader(state, block); !errors.Is(err, ErrMalformedBlock) {
		t.Fatalf("nil attestation error=%v, want ErrMalformedBlock", err)
	}
}

func TestProcessBlockHeaderValidatesProposer(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 2,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject wrong proposer")
	}
	if _, ok := err.(*InvalidProposerError); !ok {
		t.Fatalf("expected InvalidProposerError, got %T: %v", err, err)
	}
}

func TestProcessBlockHeaderValidatesParentRoot(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    [32]byte{0xff},
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject wrong parent root")
	}
	if _, ok := err.(*InvalidParentError); !ok {
		t.Fatalf("expected InvalidParentError, got %T: %v", err, err)
	}
}

func TestProcessBlockHeaderUpdatesState(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	if err := ProcessBlockHeader(state, block); err != nil {
		t.Fatal(err)
	}

	if state.LatestBlockHeader.Slot != 1 {
		t.Fatalf("expected header slot 1, got %d", state.LatestBlockHeader.Slot)
	}
	if state.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatal("new header should have zero state root")
	}

	if len(state.HistoricalBlockHashes) != 1 {
		t.Fatalf("expected 1 historical hash, got %d", len(state.HistoricalBlockHashes))
	}
}

func TestProcessBlockHeaderDoesNotMutateStateOnBodyRootError(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	parentHeader := *state.LatestBlockHeader
	justified := *state.LatestJustified
	finalized := *state.LatestFinalized
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		Body: &types.BlockBody{Attestations: []*types.AggregatedAttestation{{
			Data: &types.AttestationData{
				Head:   &types.Checkpoint{},
				Target: &types.Checkpoint{},
				Source: &types.Checkpoint{},
			},
		}}},
	}

	if err := ProcessBlockHeader(state, block); err == nil {
		t.Fatal("expected body root error")
	}
	if state.LatestBlockHeader.Slot != parentHeader.Slot ||
		state.LatestBlockHeader.StateRoot != parentHeader.StateRoot {
		t.Fatalf("latest header mutated on error: %+v before %+v", state.LatestBlockHeader, parentHeader)
	}
	if state.LatestJustified.Root != justified.Root || state.LatestJustified.Slot != justified.Slot {
		t.Fatalf("latest justified mutated on error: %+v before %+v", state.LatestJustified, justified)
	}
	if state.LatestFinalized.Root != finalized.Root || state.LatestFinalized.Slot != finalized.Slot {
		t.Fatalf("latest finalized mutated on error: %+v before %+v", state.LatestFinalized, finalized)
	}
	if len(state.HistoricalBlockHashes) != 0 {
		t.Fatalf("historical hashes mutated on error: len=%d", len(state.HistoricalBlockHashes))
	}
}

func TestProcessBlockHeaderDoesNotMutateStateOnSlotGapError(t *testing.T) {
	state := makeGenesisState(1)
	state.Slot = 1
	state.HistoricalBlockHashes = make([][]byte, types.HistoricalRootsLimit)

	parentHeader := *state.LatestBlockHeader
	justified := *state.LatestJustified
	finalized := *state.LatestFinalized
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}

	var gapErr *SlotGapTooLargeError
	if err := ProcessBlockHeader(state, block); !errors.As(err, &gapErr) {
		t.Fatalf("ProcessBlockHeader error=%v, want SlotGapTooLargeError", err)
	}
	if state.LatestBlockHeader.Slot != parentHeader.Slot ||
		state.LatestBlockHeader.StateRoot != parentHeader.StateRoot {
		t.Fatalf("latest header mutated on error: %+v before %+v", state.LatestBlockHeader, parentHeader)
	}
	if state.LatestJustified.Root != justified.Root || state.LatestJustified.Slot != justified.Slot {
		t.Fatalf("latest justified mutated on error: %+v before %+v", state.LatestJustified, justified)
	}
	if state.LatestFinalized.Root != finalized.Root || state.LatestFinalized.Slot != finalized.Slot {
		t.Fatalf("latest finalized mutated on error: %+v before %+v", state.LatestFinalized, finalized)
	}
	if len(state.HistoricalBlockHashes) != types.HistoricalRootsLimit {
		t.Fatalf("historical hashes len=%d, want %d", len(state.HistoricalBlockHashes), types.HistoricalRootsLimit)
	}
}

func TestProcessBlockHeaderRejectsOverflowSizedSlotGap(t *testing.T) {
	state := makeGenesisState(1)
	hugeSlot := ^uint64(0)
	state.Slot = hugeSlot
	state.HistoricalBlockHashes = [][]byte{make([]byte, types.RootSize)}

	parentHeader := *state.LatestBlockHeader
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          hugeSlot,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}

	var gapErr *SlotGapTooLargeError
	if err := ProcessBlockHeader(state, block); !errors.As(err, &gapErr) {
		t.Fatalf("ProcessBlockHeader error=%v, want SlotGapTooLargeError", err)
	}
	if state.LatestBlockHeader.Slot != parentHeader.Slot ||
		state.LatestBlockHeader.StateRoot != parentHeader.StateRoot {
		t.Fatalf("latest header mutated on overflow gap: %+v before %+v", state.LatestBlockHeader, parentHeader)
	}
	if len(state.HistoricalBlockHashes) != 1 {
		t.Fatalf("historical hashes len=%d, want 1", len(state.HistoricalBlockHashes))
	}
}

func TestProcessBlockHeaderSkippedSlots(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 4)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          4,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	if err := ProcessBlockHeader(state, block); err != nil {
		t.Fatal(err)
	}

	if len(state.HistoricalBlockHashes) != 4 {
		t.Fatalf("expected 4 historical hashes, got %d", len(state.HistoricalBlockHashes))
	}

	var zeroHash [32]byte
	var first [32]byte
	copy(first[:], state.HistoricalBlockHashes[0])
	if first == zeroHash {
		t.Fatal("first entry should be parent root, not zero")
	}
	for i := 1; i < 4; i++ {
		var h [32]byte
		copy(h[:], state.HistoricalBlockHashes[i])
		if h != zeroHash {
			t.Fatalf("entry %d should be zero for skipped slot", i)
		}
	}
}

func TestStateTransitionRejectsNilBlock(t *testing.T) {
	if err := StateTransition(makeGenesisState(1), nil); !errors.Is(err, ErrMalformedBlock) {
		t.Fatalf("StateTransition nil block error=%v, want ErrMalformedBlock", err)
	}
}
