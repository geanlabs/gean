package genesis

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

// HasExecutionLayer reports whether the network config declares an execution
// layer. The declaration is the genesis block hash itself; see
// ExecutionGenesisBlockHash.
func (gc *GenesisConfig) HasExecutionLayer() bool {
	return gc != nil && strings.TrimSpace(gc.ExecutionGenesisHash) != ""
}

// ExecutionGenesisBlockHash returns the execution layer's block 0 hash and
// whether one is configured. Validation at load time guarantees a configured
// value decodes.
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

// GenesisBody is the body of the genesis block. On an execution-layer network
// it carries the execution genesis hash as the payload's block hash, which is
// what fork choice reads back when it tells the execution layer where the
// chain's head, safe, and finalized blocks are. The genesis state's header
// commits to this body, so the state and the block must be built from the
// same value.
func (gc *GenesisConfig) GenesisBody() *types.BlockBody {
	hash, ok := gc.ExecutionGenesisBlockHash()
	if !ok {
		return &types.BlockBody{}
	}
	return &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockHash: hash}}
}

// genesisExecutionHeader is the cached payload header the genesis state starts
// with: zero on a pure-consensus network, otherwise a header whose block hash
// is the execution genesis so the first block's payload has something to chain
// from.
func (gc *GenesisConfig) genesisExecutionHeader() types.ExecutionPayloadHeader {
	hash, ok := gc.ExecutionGenesisBlockHash()
	if !ok {
		return types.ExecutionPayloadHeader{}
	}
	return types.ExecutionPayloadHeader{BlockHash: hash}
}
