package genesis

import (
	"bytes"
	"testing"

	"github.com/devylongs/gean/types"
)

func TestLoadFromJSON_WithValidatorCount(t *testing.T) {
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"VALIDATOR_COUNT": 100
	}`)

	config, err := LoadFromJSON(jsonData)
	if err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	if config.GenesisTime != 1704085200 {
		t.Errorf("GenesisTime = %d, want 1704085200", config.GenesisTime)
	}

	if config.ValidatorCount != 100 {
		t.Errorf("ValidatorCount = %d, want 100", config.ValidatorCount)
	}
}

func TestLoadFromJSON_WithGenesisValidators(t *testing.T) {
	// 52 bytes = 104 hex chars
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"GENESIS_VALIDATORS": [
			"0xe2a03c1689769ae5f5762222b170b4a925f3f8e89340ed1cd31d31c134b0abc2e2a03c1689769ae5f5762222b170000000000000",
			"0x0767e659c1b61d30f65eadb7a309c4183d5d4c0f99e935737b89ce95dd1c45680767e659c1b61d30f65eadb7a309000000000000"
		]
	}`)

	config, err := LoadFromJSON(jsonData)
	if err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	if config.GenesisTime != 1704085200 {
		t.Errorf("GenesisTime = %d, want 1704085200", config.GenesisTime)
	}

	// ValidatorCount should be derived from GENESIS_VALIDATORS length
	if config.ValidatorCount != 2 {
		t.Errorf("ValidatorCount = %d, want 2", config.ValidatorCount)
	}
}

func TestLoadFromJSON_ValidatorCountPrecedence(t *testing.T) {
	// When both VALIDATOR_COUNT and GENESIS_VALIDATORS are provided,
	// VALIDATOR_COUNT takes precedence
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"VALIDATOR_COUNT": 10,
		"GENESIS_VALIDATORS": [
			"0xe2a03c1689769ae5f5762222b170b4a925f3f8e89340ed1cd31d31c134b0abc2e2a03c1689769ae5f5762222b170000000000000"
		]
	}`)

	config, err := LoadFromJSON(jsonData)
	if err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	if config.ValidatorCount != 10 {
		t.Errorf("ValidatorCount = %d, want 10 (should use VALIDATOR_COUNT, not len(GENESIS_VALIDATORS))", config.ValidatorCount)
	}
}

func TestGenerateGenesis_Slot(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	if state.Slot != 0 {
		t.Errorf("Slot = %d, want 0", state.Slot)
	}
}

func TestGenerateGenesis_Config(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	if state.Config.GenesisTime != 1704085200 {
		t.Errorf("Config.GenesisTime = %d, want 1704085200", state.Config.GenesisTime)
	}

	if state.Config.NumValidators != 4 {
		t.Errorf("Config.NumValidators = %d, want 4", state.Config.NumValidators)
	}
}

func TestGenerateGenesis_Checkpoints(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	// Default checkpoints should be at slot 0 with zero root
	if state.LatestJustified.Slot != 0 {
		t.Errorf("LatestJustified.Slot = %d, want 0", state.LatestJustified.Slot)
	}
	if !state.LatestJustified.Root.IsZero() {
		t.Error("LatestJustified.Root should be zero")
	}

	if state.LatestFinalized.Slot != 0 {
		t.Errorf("LatestFinalized.Slot = %d, want 0", state.LatestFinalized.Slot)
	}
	if !state.LatestFinalized.Root.IsZero() {
		t.Error("LatestFinalized.Root should be zero")
	}
}

func TestGenerateGenesis_BlockHeader(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	header := state.LatestBlockHeader

	if header.Slot != 0 {
		t.Errorf("LatestBlockHeader.Slot = %d, want 0", header.Slot)
	}
	if header.ProposerIndex != 0 {
		t.Errorf("LatestBlockHeader.ProposerIndex = %d, want 0", header.ProposerIndex)
	}
	if !header.ParentRoot.IsZero() {
		t.Error("LatestBlockHeader.ParentRoot should be zero")
	}
	if !header.StateRoot.IsZero() {
		t.Error("LatestBlockHeader.StateRoot should be zero")
	}
	// BodyRoot should be the hash of an empty body, not zero
	if header.BodyRoot.IsZero() {
		t.Error("LatestBlockHeader.BodyRoot should not be zero (it's the hash of empty body)")
	}
}

func TestGenerateGenesis_EmptyLists(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	if len(state.HistoricalBlockHashes) != 0 {
		t.Errorf("len(HistoricalBlockHashes) = %d, want 0", len(state.HistoricalBlockHashes))
	}
	if len(state.JustificationsRoots) != 0 {
		t.Errorf("len(JustificationsRoots) = %d, want 0", len(state.JustificationsRoots))
	}
}

func TestGenerateGenesis_SSZRoundTrip(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	// Marshal
	encoded, err := state.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal
	decoded := &types.State{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	// Verify
	if decoded.Slot != state.Slot {
		t.Errorf("decoded.Slot = %d, want %d", decoded.Slot, state.Slot)
	}
	if decoded.Config.GenesisTime != state.Config.GenesisTime {
		t.Errorf("decoded.Config.GenesisTime = %d, want %d", decoded.Config.GenesisTime, state.Config.GenesisTime)
	}
	if decoded.Config.NumValidators != state.Config.NumValidators {
		t.Errorf("decoded.Config.NumValidators = %d, want %d", decoded.Config.NumValidators, state.Config.NumValidators)
	}
}

func TestGenerateGenesis_HashTreeRoot(t *testing.T) {
	state := GenerateGenesis(1704085200, 4)

	root1, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	// Same state should produce the same root
	root2, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed second time: %v", err)
	}

	if !bytes.Equal(root1[:], root2[:]) {
		t.Error("HashTreeRoot is not deterministic")
	}

	// Root should not be zero
	if root1 == (types.Root{}) {
		t.Error("HashTreeRoot should not be zero")
	}
}

func TestCreateState(t *testing.T) {
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"VALIDATOR_COUNT": 4
	}`)

	config, err := LoadFromJSON(jsonData)
	if err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	state, err := config.CreateState()
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	if state.Slot != 0 {
		t.Errorf("Slot = %d, want 0", state.Slot)
	}
	if state.Config.NumValidators != 4 {
		t.Errorf("Config.NumValidators = %d, want 4", state.Config.NumValidators)
	}
	if state.Config.GenesisTime != 1704085200 {
		t.Errorf("Config.GenesisTime = %d, want 1704085200", state.Config.GenesisTime)
	}
}
