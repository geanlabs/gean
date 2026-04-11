package genesis

import (
	"os"
	"testing"

	"github.com/geanlabs/gean/types"
)

const testConfigYAML = `GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
    - "cd323f232b34ab26d6db7402c886e74ca81cfd3a0c659d2fe022356f25592f7d2d25ca7b19604f5a180037046cf2a02e1da4a800"
    - "b7b0f72e24801b02bda64073cb4de6699a416b37dfead227d7ca3922647c940fa03e4c012e8a0e656b731934aeac124a5337e333"
    - "8d9cbc508b20ef43e165f8559c1bdd18aaeda805ef565a4f9ffd6e4fbed01c05e143e305017847445859650d6dd06e6efb3f8410"
`

func TestLoadGenesisConfig(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	os.WriteFile(tmpFile, []byte(testConfigYAML), 0644)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.GenesisTime != 1770407233 {
		t.Fatalf("genesis time: expected 1770407233, got %d", config.GenesisTime)
	}
	if len(config.GenesisValidators) != 3 {
		t.Fatalf("validators: expected 3, got %d", len(config.GenesisValidators))
	}
}

func TestValidators(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	os.WriteFile(tmpFile, []byte(testConfigYAML), 0644)

	config, _ := LoadGenesisConfig(tmpFile)
	validators := config.Validators()

	if len(validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(validators))
	}
	for i, v := range validators {
		if v.Index != uint64(i) {
			t.Fatalf("validator %d index: expected %d, got %d", i, i, v.Index)
		}
		if v.AttestationPubkey == [types.PubkeySize]byte{} {
			t.Fatalf("validator %d has zero attestation pubkey", i)
		}
	}
}

func TestGenesisState(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	os.WriteFile(tmpFile, []byte(testConfigYAML), 0644)

	config, _ := LoadGenesisConfig(tmpFile)
	state := config.GenesisState()

	if state.Slot != 0 {
		t.Fatalf("genesis slot should be 0, got %d", state.Slot)
	}
	if state.Config.GenesisTime != 1770407233 {
		t.Fatal("genesis time mismatch")
	}
	if len(state.Validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(state.Validators))
	}
	if !types.IsZeroRoot(state.LatestJustified.Root) {
		t.Fatal("justified root should be zero at genesis")
	}
	if !types.IsZeroRoot(state.LatestFinalized.Root) {
		t.Fatal("finalized root should be zero at genesis")
	}
}

func TestLoadGenesisConfigMissing(t *testing.T) {
	_, err := LoadGenesisConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("should error on missing file")
	}
}
