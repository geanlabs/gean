package statetransition

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

const testGenesisTime = 1_700_000_000

func executionState(slot uint64, lastHash [32]byte) *types.State {
	return &types.State{
		Config:                       &types.ChainConfig{GenesisTime: testGenesisTime},
		Slot:                         slot,
		LatestExecutionPayloadHeader: types.ExecutionPayloadHeader{BlockHash: lastHash},
	}
}

func executionBlock(slot uint64, payload *types.ExecutionPayload) *types.Block {
	body := &types.BlockBody{}
	if payload != nil {
		body.ExecutionPayload = *payload
	}
	return &types.Block{Slot: slot, Body: body}
}

func TestProcessExecutionPayload(t *testing.T) {
	elGenesis := [32]byte{0xE1}
	next := [32]byte{0xE2}
	valid := &types.ExecutionPayload{
		ParentHash: elGenesis,
		Timestamp:  ComputeTimeAtSlot(testGenesisTime, 3),
		BlockHash:  next,
	}

	tests := []struct {
		name    string
		state   *types.State
		block   *types.Block
		wantErr error
		wantAs  any
	}{
		{
			name:  "pure consensus chain accepts a nil payload",
			state: executionState(3, types.ZeroRoot),
			block: executionBlock(3, nil),
		},
		{
			name:  "pure consensus chain accepts a zero payload",
			state: executionState(3, types.ZeroRoot),
			block: executionBlock(3, &types.ExecutionPayload{}),
		},
		{
			name:    "pure consensus chain rejects a payload",
			state:   executionState(3, types.ZeroRoot),
			block:   executionBlock(3, valid),
			wantErr: ErrUnexpectedExecutionPayload,
		},
		{
			name:  "execution chain accepts a chained payload",
			state: executionState(3, elGenesis),
			block: executionBlock(3, valid),
		},
		{
			name:    "execution chain rejects a missing payload",
			state:   executionState(3, elGenesis),
			block:   executionBlock(3, nil),
			wantErr: ErrMissingExecutionPayload,
		},
		{
			name:   "execution chain rejects a parent hash mismatch",
			state:  executionState(3, elGenesis),
			block:  executionBlock(3, &types.ExecutionPayload{ParentHash: [32]byte{0xFF}, Timestamp: valid.Timestamp, BlockHash: next}),
			wantAs: &ExecutionPayloadParentHashError{},
		},
		{
			name:   "execution chain rejects a timestamp off by one slot",
			state:  executionState(3, elGenesis),
			block:  executionBlock(3, &types.ExecutionPayload{ParentHash: elGenesis, Timestamp: ComputeTimeAtSlot(testGenesisTime, 4), BlockHash: next}),
			wantAs: &ExecutionPayloadTimestampError{},
		},
		{
			name:    "execution chain rejects a zero block hash",
			state:   executionState(3, elGenesis),
			block:   executionBlock(3, &types.ExecutionPayload{ParentHash: elGenesis, Timestamp: valid.Timestamp}),
			wantErr: ErrZeroExecutionBlockHash,
		},
		{
			name:    "zero header on an execution-free state means no execution layer",
			state:   &types.State{Config: &types.ChainConfig{GenesisTime: testGenesisTime}, Slot: 3},
			block:   executionBlock(3, valid),
			wantErr: ErrUnexpectedExecutionPayload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ProcessExecutionPayload(tt.state, tt.block)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got %v, want %v", err, tt.wantErr)
				}
			case tt.wantAs != nil:
				switch target := tt.wantAs.(type) {
				case *ExecutionPayloadParentHashError:
					if !errors.As(err, &target) {
						t.Fatalf("got %v, want parent hash error", err)
					}
				case *ExecutionPayloadTimestampError:
					if !errors.As(err, &target) {
						t.Fatalf("got %v, want timestamp error", err)
					}
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestProcessExecutionPayloadCachesHeaderAndChains(t *testing.T) {
	elGenesis := [32]byte{0xE1}
	state := executionState(1, elGenesis)

	first := &types.ExecutionPayload{
		ParentHash:   elGenesis,
		Timestamp:    ComputeTimeAtSlot(testGenesisTime, 1),
		BlockHash:    [32]byte{0xA1},
		Transactions: [][]byte{{0x01}},
	}
	if err := ProcessExecutionPayload(state, executionBlock(1, first)); err != nil {
		t.Fatalf("first block: %v", err)
	}
	wantHeader, _ := first.ToHeader()
	if state.LatestExecutionPayloadHeader.BlockHash != first.BlockHash ||
		state.LatestExecutionPayloadHeader.TransactionsRoot != wantHeader.TransactionsRoot {
		t.Fatal("header was not cached from the applied payload")
	}

	state.Slot = 2
	second := &types.ExecutionPayload{
		ParentHash: first.BlockHash,
		Timestamp:  ComputeTimeAtSlot(testGenesisTime, 2),
		BlockHash:  [32]byte{0xA2},
	}
	if err := ProcessExecutionPayload(state, executionBlock(2, second)); err != nil {
		t.Fatalf("second block: %v", err)
	}
	if state.LatestExecutionPayloadHeader.BlockHash != second.BlockHash {
		t.Fatal("header did not advance to the second payload")
	}
}

func TestComputeTimeAtSlot(t *testing.T) {
	if got := ComputeTimeAtSlot(100, 0); got != 100 {
		t.Fatalf("slot 0: got %d", got)
	}
	if got := ComputeTimeAtSlot(100, 5); got != 100+5*types.SecondsPerSlot {
		t.Fatalf("slot 5: got %d", got)
	}
}
