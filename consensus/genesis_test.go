package consensus

import (
	"testing"

	"github.com/devylongs/gean/types"
)

func TestGenerateGenesis_Fields(t *testing.T) {
	genesisTime := uint64(1000000000)
	numValidators := uint64(8)

	state, block := GenerateGenesis(genesisTime, numValidators)

	if state.Config.GenesisTime != genesisTime {
		t.Errorf("genesis time = %d, want %d", state.Config.GenesisTime, genesisTime)
	}
	if state.Config.NumValidators != numValidators {
		t.Errorf("num validators = %d, want %d", state.Config.NumValidators, numValidators)
	}
	if state.Slot != 0 {
		t.Errorf("slot = %d, want 0", state.Slot)
	}
	if state.LatestBlockHeader.Slot != 0 {
		t.Errorf("latest block header slot = %d, want 0", state.LatestBlockHeader.Slot)
	}
	if state.LatestJustified.Slot != 0 {
		t.Errorf("latest justified slot = %d, want 0", state.LatestJustified.Slot)
	}
	if state.LatestFinalized.Slot != 0 {
		t.Errorf("latest finalized slot = %d, want 0", state.LatestFinalized.Slot)
	}
	if !state.LatestJustified.Root.IsZero() {
		t.Error("latest justified root should be zero")
	}
	if !state.LatestFinalized.Root.IsZero() {
		t.Error("latest finalized root should be zero")
	}
	if len(state.HistoricalBlockHashes) != 0 {
		t.Errorf("historical hashes length = %d, want 0", len(state.HistoricalBlockHashes))
	}

	if block.Slot != 0 {
		t.Errorf("block slot = %d, want 0", block.Slot)
	}
	if block.ProposerIndex != 0 {
		t.Errorf("block proposer = %d, want 0", block.ProposerIndex)
	}
	if !block.ParentRoot.IsZero() {
		t.Error("block parent root should be zero")
	}
}

func TestGenerateGenesis_BlockStateRoot(t *testing.T) {
	state, block := GenerateGenesis(1000000000, 8)

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash state: %v", err)
	}

	if block.StateRoot != stateRoot {
		t.Errorf("block state root does not match state hash")
	}
}

func TestGenerateGenesis_SSZRoundtrip(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, 8)

	data, err := state.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.State
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	originalRoot, _ := state.HashTreeRoot()
	decodedRoot, _ := decoded.HashTreeRoot()

	if originalRoot != decodedRoot {
		t.Error("SSZ roundtrip hash mismatch")
	}
}
