package genesis

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

// HasExecutionLayer reports whether an execution genesis hash is configured.
func (gc *GenesisConfig) HasExecutionLayer() bool {
	return gc != nil && strings.TrimSpace(gc.ExecutionGenesisHash) != ""
}

// ExecutionGenesisBlockHash returns the configured block 0 hash, validated at load time.
func (gc *GenesisConfig) ExecutionGenesisBlockHash() ([32]byte, bool) {
	hash, ok, err := gc.executionGenesisBlockHash()
	if err != nil {
		return [32]byte{}, false
	}
	return hash, ok
}

func (gc *GenesisConfig) executionGenesisBlockHash() ([32]byte, bool, error) {
	if !gc.HasExecutionLayer() {
		return [32]byte{}, false, nil
	}
	normalized := strings.TrimSpace(gc.ExecutionGenesisHash)
	normalized = strings.TrimPrefix(strings.TrimPrefix(normalized, "0x"), "0X")
	raw, err := hex.DecodeString(normalized)
	if err != nil || len(raw) != types.RootSize {
		return [32]byte{}, false, fmt.Errorf("EXECUTION_GENESIS_BLOCK_HASH must be 32 bytes of hex: %q", gc.ExecutionGenesisHash)
	}
	var hash [32]byte
	copy(hash[:], raw)
	if types.IsZeroRoot(hash) {
		return [32]byte{}, false, fmt.Errorf("EXECUTION_GENESIS_BLOCK_HASH must not be zero")
	}
	return hash, true, nil
}

// GenesisBody commits the execution genesis hash so fork choice can resolve it.
func (gc *GenesisConfig) GenesisBody() *types.BlockBody {
	hash, _ := gc.ExecutionGenesisBlockHash()
	return &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockHash: hash}}
}

// genesisExecutionHeader anchors the first payload, or is zero on consensus-only networks.
func (gc *GenesisConfig) genesisExecutionHeader() types.ExecutionPayloadHeader {
	hash, _ := gc.ExecutionGenesisBlockHash()
	return types.ExecutionPayloadHeader{BlockHash: hash}
}
