package checkpoint

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func makeTestState(slot uint64, genesisTime uint64, numValidators int) *types.State {
	validators := make([]*types.Validator, numValidators)
	for i := 0; i < numValidators; i++ {
		validators[i] = &types.Validator{
			Pubkey: [types.PubkeySize]byte{byte(i + 1)},
			Index:  uint64(i),
		}
	}

	header := &types.BlockHeader{Slot: slot}

	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: genesisTime},
		Slot:                     slot,
		LatestBlockHeader:        header,
		LatestJustified:          &types.Checkpoint{Slot: slot - 2},
		LatestFinalized:          &types.Checkpoint{Slot: slot - 5},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestVerifyCheckpointStateValid(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	expectedValidators := state.Validators

	err := VerifyCheckpointState(state, 1000, expectedValidators)
	if err != nil {
		t.Fatalf("should pass: %v", err)
	}
}

func TestVerifyCheckpointStateSlotZero(t *testing.T) {
	state := makeTestState(0, 1000, 3)
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: slot is 0")
	}
}

func TestVerifyCheckpointStateNoValidators(t *testing.T) {
	state := makeTestState(100, 1000, 0)
	err := VerifyCheckpointState(state, 1000, nil)
	if err == nil {
		t.Fatal("should fail: no validators")
	}
}

func TestVerifyCheckpointStateGenesisTimeMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	err := VerifyCheckpointState(state, 9999, state.Validators)
	if err == nil {
		t.Fatal("should fail: genesis time mismatch")
	}
}

func TestVerifyCheckpointStateValidatorCountMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	twoValidators := state.Validators[:2]
	err := VerifyCheckpointState(state, 1000, twoValidators)
	if err == nil {
		t.Fatal("should fail: validator count mismatch")
	}
}

func TestVerifyCheckpointStateNonSequentialIndex(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.Validators[1].Index = 99 // break sequential
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: non-sequential index")
	}
}

func TestVerifyCheckpointStatePubkeyMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	// Different expected validators.
	expected := make([]*types.Validator, 3)
	for i := 0; i < 3; i++ {
		expected[i] = &types.Validator{
			Pubkey: [types.PubkeySize]byte{byte(i + 100)}, // different
			Index:  uint64(i),
		}
	}
	err := VerifyCheckpointState(state, 1000, expected)
	if err == nil {
		t.Fatal("should fail: pubkey mismatch")
	}
}

func TestVerifyCheckpointStateFinalizedExceedsState(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestFinalized.Slot = 200 // > state.Slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: finalized exceeds state")
	}
}

func TestVerifyCheckpointStateJustifiedPrecedesFinalized(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestJustified.Slot = 90
	state.LatestFinalized.Slot = 95 // justified < finalized
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: justified precedes finalized")
	}
}

func TestVerifyCheckpointStateJustifiedFinalizedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestJustified.Slot = 50
	state.LatestFinalized.Slot = 50
	state.LatestJustified.Root = [32]byte{1}
	state.LatestFinalized.Root = [32]byte{2} // different roots at same slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: root mismatch at same slot")
	}
}

func TestVerifyCheckpointStateBlockHeaderExceedsState(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 200 // > state.Slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header exceeds state")
	}
}

func TestVerifyCheckpointStateBlockHeaderFinalizedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 50
	state.LatestFinalized.Slot = 50
	state.LatestFinalized.Root = [32]byte{99} // wrong root
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header root mismatch at finalized slot")
	}
}

func TestVerifyCheckpointStateBlockHeaderJustifiedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 90
	state.LatestJustified.Slot = 90
	state.LatestJustified.Root = [32]byte{99} // wrong root
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header root mismatch at justified slot")
	}
}
