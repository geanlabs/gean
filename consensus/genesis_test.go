package consensus

import (
	"testing"

	"github.com/devylongs/gean/types"
)

// makeTestValidators creates n placeholder validators for testing.
func makeTestValidators(n uint64) []types.Validator {
	validators := make([]types.Validator, n)
	for i := uint64(0); i < n; i++ {
		validators[i] = types.Validator{Index: types.ValidatorIndex(i)}
	}
	return validators
}

func TestGenerateValidators_DeterministicAndIndexed(t *testing.T) {
	v1 := GenerateValidators(4)
	v2 := GenerateValidators(4)

	if len(v1) != 4 || len(v2) != 4 {
		t.Fatalf("unexpected validator count: %d %d", len(v1), len(v2))
	}
	for i := range v1 {
		if v1[i].Index != types.ValidatorIndex(i) {
			t.Fatalf("validator index mismatch at %d: got %d", i, v1[i].Index)
		}
		if v2[i].Index != types.ValidatorIndex(i) {
			t.Fatalf("validator index mismatch (second run) at %d: got %d", i, v2[i].Index)
		}
		if v1[i].Pubkey != v2[i].Pubkey {
			t.Fatalf("determinism mismatch at validator %d", i)
		}
	}
	if v1[0].Pubkey == (types.Pubkey{}) {
		t.Fatal("expected non-zero deterministic placeholder pubkey")
	}
}

func TestGenerateValidators_EmptyForNonPositive(t *testing.T) {
	if got := GenerateValidators(0); len(got) != 0 {
		t.Fatalf("expected empty validators for n=0, got %d", len(got))
	}
	if got := GenerateValidators(-1); len(got) != 0 {
		t.Fatalf("expected empty validators for n<0, got %d", len(got))
	}
}

func TestGenerateGenesis_Fields(t *testing.T) {
	genesisTime := uint64(1000000000)
	validators := GenerateValidators(8)

	state, block := GenerateGenesis(genesisTime, validators)

	if state.Config.GenesisTime != genesisTime {
		t.Errorf("genesis time = %d, want %d", state.Config.GenesisTime, genesisTime)
	}
	if len(state.Validators) != 8 {
		t.Errorf("validators length = %d, want 8", len(state.Validators))
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
	state, block := GenerateGenesis(1000000000, GenerateValidators(8))

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash state: %v", err)
	}

	if block.StateRoot != stateRoot {
		t.Errorf("block state root does not match state hash")
	}
}

func TestGenerateGenesis_SSZRoundtrip(t *testing.T) {
	state, _ := GenerateGenesis(1000000000, GenerateValidators(8))

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

func TestGenerateGenesis_CopiesValidatorSlice(t *testing.T) {
	validators := GenerateValidators(2)
	state, _ := GenerateGenesis(1000000000, validators)

	// Mutate caller slice and verify state keeps its own copy.
	validators[0].Index = 99
	if state.Validators[0].Index == 99 {
		t.Fatal("state validators alias caller slice")
	}
}
