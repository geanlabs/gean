package genesis

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/devylongs/gean/types"
)

func TestLoadFromJSON(t *testing.T) {
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

	if len(config.GenesisValidators) != 2 {
		t.Errorf("len(GenesisValidators) = %d, want 2", len(config.GenesisValidators))
	}
}

func TestLoadFromJSON_InvalidHex(t *testing.T) {
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"GENESIS_VALIDATORS": ["invalid-hex"]
	}`)

	_, err := LoadFromJSON(jsonData)
	if err == nil {
		t.Error("expected error for invalid hex, got nil")
	}
}

func TestLoadFromJSON_WrongLength(t *testing.T) {
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"GENESIS_VALIDATORS": ["0x1234"]
	}`)

	_, err := LoadFromJSON(jsonData)
	if err == nil {
		t.Error("expected error for wrong length pubkey, got nil")
	}
}

func TestToValidators(t *testing.T) {
	var pubkey1, pubkey2 types.Bytes52
	hex.Decode(pubkey1[:], []byte("e2a03c1689769ae5f5762222b170b4a925f3f8e89340ed1cd31d31c134b0abc2e2a03c1689769ae5f5762222b17000"))
	hex.Decode(pubkey2[:], []byte("0767e659c1b61d30f65eadb7a309c4183d5d4c0f99e935737b89ce95dd1c45680767e659c1b61d30f65eadb7a30900"))

	config := &GenesisConfig{
		GenesisTime:       1704085200,
		GenesisValidators: []types.Bytes52{pubkey1, pubkey2},
	}

	validators := config.ToValidators()

	if len(validators) != 2 {
		t.Fatalf("len(validators) = %d, want 2", len(validators))
	}

	if validators[0].Index != 0 {
		t.Errorf("validators[0].Index = %d, want 0", validators[0].Index)
	}
	if validators[1].Index != 1 {
		t.Errorf("validators[1].Index = %d, want 1", validators[1].Index)
	}

	if validators[0].Pubkey != pubkey1 {
		t.Error("validators[0].Pubkey does not match")
	}
}

func TestGenerateGenesis_Slot(t *testing.T) {
	validators := []types.Validator{
		{Pubkey: types.Bytes52{}, Index: 0},
	}

	state := GenerateGenesis(1704085200, validators)

	if state.Slot != 0 {
		t.Errorf("Slot = %d, want 0", state.Slot)
	}
}

func TestGenerateGenesis_Validators(t *testing.T) {
	var pubkey types.Bytes52
	pubkey[0] = 0xAB

	validators := []types.Validator{
		{Pubkey: pubkey, Index: 0},
		{Pubkey: types.Bytes52{}, Index: 1},
	}

	state := GenerateGenesis(1704085200, validators)

	if len(state.Validators) != 2 {
		t.Fatalf("len(Validators) = %d, want 2", len(state.Validators))
	}

	if state.Validators[0].Pubkey[0] != 0xAB {
		t.Error("validator pubkey not preserved")
	}
}

func TestGenerateGenesis_Config(t *testing.T) {
	state := GenerateGenesis(1704085200, nil)

	if state.Config.GenesisTime != 1704085200 {
		t.Errorf("Config.GenesisTime = %d, want 1704085200", state.Config.GenesisTime)
	}
}

func TestGenerateGenesis_Checkpoints(t *testing.T) {
	state := GenerateGenesis(1704085200, nil)

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
	state := GenerateGenesis(1704085200, nil)

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
	state := GenerateGenesis(1704085200, nil)

	if len(state.HistoricalRoots) != 0 {
		t.Errorf("len(HistoricalRoots) = %d, want 0", len(state.HistoricalRoots))
	}
	if len(state.JustificationRoots) != 0 {
		t.Errorf("len(JustificationRoots) = %d, want 0", len(state.JustificationRoots))
	}
}

func TestGenerateGenesis_SSZRoundTrip(t *testing.T) {
	validators := []types.Validator{
		{Pubkey: types.Bytes52{0x01}, Index: 0},
	}

	state := GenerateGenesis(1704085200, validators)

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
	if len(decoded.Validators) != len(state.Validators) {
		t.Errorf("len(decoded.Validators) = %d, want %d", len(decoded.Validators), len(state.Validators))
	}
}

func TestGenerateGenesis_HashTreeRoot(t *testing.T) {
	validators := []types.Validator{
		{Pubkey: types.Bytes52{0x01}, Index: 0},
	}

	state := GenerateGenesis(1704085200, validators)

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
	// 52 bytes = 104 hex chars
	jsonData := []byte(`{
		"GENESIS_TIME": 1704085200,
		"GENESIS_VALIDATORS": [
			"0xe2a03c1689769ae5f5762222b170b4a925f3f8e89340ed1cd31d31c134b0abc2e2a03c1689769ae5f5762222b170000000000000"
		]
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
	if len(state.Validators) != 1 {
		t.Errorf("len(Validators) = %d, want 1", len(state.Validators))
	}
	if state.Config.GenesisTime != 1704085200 {
		t.Errorf("Config.GenesisTime = %d, want 1704085200", state.Config.GenesisTime)
	}
}
