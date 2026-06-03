package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestTryFinalizeAdvancesAndPrunesJustifications(t *testing.T) {
	state := makeGenesisState(1)
	state.LatestFinalized = &types.Checkpoint{Slot: 0}
	state.JustifiedSlots = types.NewBitlistSSZ(8)
	types.BitlistSet(state.JustifiedSlots, 5)

	sourceRoot := [32]byte{0x05}
	targetRoot := [32]byte{0x06}
	oldRoot := [32]byte{0x04}
	futureRoot := [32]byte{0x08}

	justifications := map[[32]byte][]bool{
		oldRoot:    {true},
		futureRoot: {true},
	}
	rootToSlot := map[[32]byte]uint64{
		oldRoot:    4,
		futureRoot: 8,
	}

	tryFinalize(
		state,
		&types.Checkpoint{Slot: 5, Root: sourceRoot},
		&types.Checkpoint{Slot: 6, Root: targetRoot},
		&justifications,
		rootToSlot,
	)

	if state.LatestFinalized.Slot != 5 || state.LatestFinalized.Root != sourceRoot {
		t.Fatalf("latest finalized=%+v, want slot 5 root %x", state.LatestFinalized, sourceRoot)
	}
	if _, ok := justifications[oldRoot]; ok {
		t.Fatal("old justification was not pruned")
	}
	if _, ok := justifications[futureRoot]; !ok {
		t.Fatal("future justification was pruned")
	}
	if types.BitlistLen(state.JustifiedSlots) != 3 || !types.BitlistGet(state.JustifiedSlots, 0) {
		t.Fatalf("justified slots were not shifted as expected: len=%d bits=%08b",
			types.BitlistLen(state.JustifiedSlots), state.JustifiedSlots)
	}
}

func TestTryFinalizeKeepsStateWhenGapIsJustifiable(t *testing.T) {
	state := makeGenesisState(1)
	state.LatestFinalized = &types.Checkpoint{Slot: 0}
	justifications := map[[32]byte][]bool{}

	tryFinalize(
		state,
		&types.Checkpoint{Slot: 1, Root: [32]byte{0x01}},
		&types.Checkpoint{Slot: 7, Root: [32]byte{0x07}},
		&justifications,
		nil,
	)

	if state.LatestFinalized.Slot != 0 {
		t.Fatalf("latest finalized slot=%d, want 0", state.LatestFinalized.Slot)
	}
}

func TestTryFinalizeIgnoresStaleFinalizedSource(t *testing.T) {
	state := makeGenesisState(1)
	state.LatestFinalized = &types.Checkpoint{Slot: 4, Root: [32]byte{0x04}}
	state.LatestJustified = &types.Checkpoint{Slot: 6, Root: [32]byte{0x06}}
	state.JustifiedSlots = types.NewBitlistSSZ(2)
	types.BitlistSet(state.JustifiedSlots, 1)

	justifications := map[[32]byte][]bool{
		{0x06}: {true},
	}

	tryFinalize(
		state,
		&types.Checkpoint{Slot: 1, Root: [32]byte{0x01}},
		&types.Checkpoint{Slot: 6, Root: [32]byte{0x06}},
		&justifications,
		nil,
	)

	if state.LatestFinalized.Slot != 4 || state.LatestFinalized.Root != [32]byte{0x04} {
		t.Fatalf("latest finalized=%+v, want slot 4", state.LatestFinalized)
	}
	if types.BitlistLen(state.JustifiedSlots) != 2 || !types.BitlistGet(state.JustifiedSlots, 1) {
		t.Fatalf("justified slots changed: len=%d bits=%08b",
			types.BitlistLen(state.JustifiedSlots), state.JustifiedSlots)
	}
}

func TestShiftJustifiedSlotsClearsWhenDeltaCoversWindow(t *testing.T) {
	state := makeGenesisState(1)
	state.JustifiedSlots = types.NewBitlistSSZ(2)
	types.BitlistSet(state.JustifiedSlots, 1)

	shiftJustifiedSlots(state, 2)

	if types.BitlistLen(state.JustifiedSlots) != 0 {
		t.Fatalf("justified slots len=%d, want 0", types.BitlistLen(state.JustifiedSlots))
	}
}
