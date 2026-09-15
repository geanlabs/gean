package statetransition

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

// ComputeTimeAtSlot is the wall-clock second the chain assigns to a slot:
// genesis plus SecondsPerSlot per slot. It is the timestamp an execution
// payload must carry to belong to that slot.
func ComputeTimeAtSlot(genesisTime, slot uint64) uint64 {
	return genesisTime + slot*types.SecondsPerSlot
}

// ProcessExecutionPayload validates the block's execution payload against the
// header the state last applied and caches the new header.
//
// Whether the chain has an execution layer is committed in the genesis state:
// a zero BlockHash in the cached header means it has none, and every block must
// then carry the zero payload. Otherwise the payload must chain from the cached
// header's BlockHash and carry the timestamp of the block's slot. The
// execution-layer roundtrip that decides whether the payload executes belongs
// to the import path, not here; this function runs in fixture replay and block
// production where no execution layer is reachable.
func ProcessExecutionPayload(state *types.State, block *types.Block) error {
	if state == nil {
		return malformedState("state")
	}
	if state.Config == nil {
		return malformedState("config")
	}
	if block == nil {
		return malformedBlock("block")
	}
	if block.Body == nil {
		return malformedBlock("body")
	}

	payload := &block.Body.ExecutionPayload
	header := &state.LatestExecutionPayloadHeader

	if types.IsZeroRoot(header.BlockHash) {
		if !payload.IsZero() {
			return ErrUnexpectedExecutionPayload
		}
		return nil
	}

	if payload.IsZero() {
		return ErrMissingExecutionPayload
	}
	if payload.ParentHash != header.BlockHash {
		return &ExecutionPayloadParentHashError{Expected: header.BlockHash, Found: payload.ParentHash}
	}
	expectedTimestamp := ComputeTimeAtSlot(state.Config.GenesisTime, state.Slot)
	if payload.Timestamp != expectedTimestamp {
		return &ExecutionPayloadTimestampError{Expected: expectedTimestamp, Found: payload.Timestamp}
	}
	if types.IsZeroRoot(payload.BlockHash) {
		return ErrZeroExecutionBlockHash
	}
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return err
	}

	newHeader, err := payload.ToHeader()
	if err != nil {
		return fmt.Errorf("execution payload header: %w", err)
	}
	state.LatestExecutionPayloadHeader = *newHeader
	return nil
}

type ExecutionPayloadParentHashError struct {
	Expected [32]byte
	Found    [32]byte
}

func (e *ExecutionPayloadParentHashError) Error() string {
	return fmt.Sprintf("execution payload parent hash %x does not extend last applied block %x", e.Found[:4], e.Expected[:4])
}

type ExecutionPayloadTimestampError struct {
	Expected uint64
	Found    uint64
}

func (e *ExecutionPayloadTimestampError) Error() string {
	return fmt.Sprintf("execution payload timestamp %d != slot time %d", e.Found, e.Expected)
}

var ErrUnexpectedExecutionPayload = fmt.Errorf("execution payload present on a chain without an execution layer")
var ErrMissingExecutionPayload = fmt.Errorf("execution payload missing on a chain with an execution layer")
var ErrZeroExecutionBlockHash = fmt.Errorf("execution payload block hash is zero")
