package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

func TestSnapshotInputsCapturesPayloadAndTargetState(t *testing.T) {
	s := pools.New()

	var headRoot [32]byte
	headRoot[0] = 1
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     1,
		LatestBlockHeader:        &types.BlockHeader{Slot: 1},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}

	attData := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: headRoot, Slot: 1},
		Target: &types.Checkpoint{Root: headRoot, Slot: 1},
		Source: &types.Checkpoint{Root: headRoot, Slot: 0},
	}
	dataRoot, _ := attData.HashTreeRoot()
	participants := types.NewBitlistSSZ(1)
	types.BitlistSet(participants, 0)
	s.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{
		Participants: participants,
		Proof:        []byte{1},
	})

	snap := SnapshotInputs(s, headState, headRoot, 1)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.headState == nil || snap.headState.Slot != headState.Slot {
		t.Fatal("head state not captured")
	}
	if snap.newEntries[dataRoot] == nil {
		t.Fatal("new payload entry not captured")
	}
}

func TestSnapshotInputsReturnsNilWithoutWork(t *testing.T) {
	s := pools.New()
	if snap := SnapshotInputs(s, &types.State{}, [32]byte{}, 0); snap != nil {
		t.Fatalf("snapshot=%v, want nil", snap)
	}
	// A caller with no head state has nothing to resolve signers against.
	if snap := SnapshotInputs(s, nil, [32]byte{}, 0); snap != nil {
		t.Fatalf("snapshot=%v, want nil without a head state", snap)
	}
}
