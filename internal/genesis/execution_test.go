package genesis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

const testExecutionGenesisHash = "0xb923444291dd386fd99c90376a7660b31879ef73b212c8c92ba85147185fc7af"

func executionConfigYAML() string {
	return "EXECUTION_GENESIS_BLOCK_HASH: \"" + testExecutionGenesisHash + "\"\n" + testConfigYAML
}

func TestGenesisWithoutExecutionLayer(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)
	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.HasExecutionLayer() {
		t.Fatal("no execution layer expected")
	}
	if _, ok := config.ExecutionGenesisBlockHash(); ok {
		t.Fatal("no execution genesis hash expected")
	}
	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}
	if !types.IsZeroRoot(state.LatestExecutionPayloadHeader.BlockHash) {
		t.Fatal("pure-consensus genesis must start with a zero execution header")
	}
	if !config.GenesisBody().ExecutionPayload.IsZero() {
		t.Fatal("pure-consensus genesis body must carry a zero payload")
	}
}

func TestGenesisWithExecutionLayer(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, executionConfigYAML())
	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !config.HasExecutionLayer() {
		t.Fatal("execution layer expected")
	}
	hash, ok := config.ExecutionGenesisBlockHash()
	if !ok {
		t.Fatal("execution genesis hash expected")
	}
	if got := fmt.Sprintf("0x%x", hash); got != testExecutionGenesisHash {
		t.Fatalf("hash: got %s want %s", got, testExecutionGenesisHash)
	}

	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}
	if state.LatestExecutionPayloadHeader.BlockHash != hash {
		t.Fatal("genesis state header must carry the execution genesis hash")
	}

	// The state header commits to the genesis body, and the body must carry
	// the same hash so fork choice can resolve the genesis root to it.
	body := config.GenesisBody()
	if body.ExecutionPayload.BlockHash != hash {
		t.Fatal("genesis body must carry the execution genesis hash")
	}
	bodyRoot, err := body.HashTreeRoot()
	if err != nil {
		t.Fatalf("body root: %v", err)
	}
	if state.LatestBlockHeader.BodyRoot != bodyRoot {
		t.Fatal("genesis header body root does not match the genesis body")
	}

	// Two nodes with the same config must agree on the genesis block root.
	other, _ := config.GenesisState()
	a, _ := state.LatestBlockHeader.HashTreeRoot()
	b, _ := other.LatestBlockHeader.HashTreeRoot()
	if a != b {
		t.Fatal("genesis root is not deterministic")
	}
	plain, _ := (&GenesisConfig{GenesisTime: config.GenesisTime, GenesisValidators: config.GenesisValidators}).GenesisState()
	c, _ := plain.LatestBlockHeader.HashTreeRoot()
	if a == c {
		t.Fatal("execution genesis hash must change the genesis root")
	}
}

func TestExecutionGenesisBlockHashValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash string
	}{
		{"not hex", "0xzz"},
		{"short", "0x1234"},
		{"zero", "0x" + strings.Repeat("00", 32)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/config.yaml"
			writeGenesisConfig(t, tmpFile, "EXECUTION_GENESIS_BLOCK_HASH: \""+tt.hash+"\"\n"+testConfigYAML)
			if _, err := LoadGenesisConfig(tmpFile); err == nil {
				t.Fatalf("expected %s hash to be rejected", tt.name)
			}
		})
	}
}
