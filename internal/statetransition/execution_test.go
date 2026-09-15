package statetransition

import (
	"errors"
	"reflect"
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

	badParent, badTime, zeroHash := *valid, *valid, *valid
	badParent.ParentHash = [32]byte{0xFF}
	badTime.Timestamp = ComputeTimeAtSlot(testGenesisTime, 4)
	zeroHash.BlockHash = types.ZeroRoot
	for _, tt := range []struct {
		name    string
		parent  [32]byte
		payload *types.ExecutionPayload
		wantErr error
		wantAs  any
	}{
		{"pure consensus", types.ZeroRoot, nil, nil, nil},
		{"unexpected payload", types.ZeroRoot, valid, ErrUnexpectedExecutionPayload, nil},
		{"chained payload", elGenesis, valid, nil, nil},
		{"missing payload", elGenesis, nil, ErrMissingExecutionPayload, nil},
		{"wrong parent", elGenesis, &badParent, nil, new(*ExecutionPayloadParentHashError)},
		{"wrong timestamp", elGenesis, &badTime, nil, new(*ExecutionPayloadTimestampError)},
		{"zero block hash", elGenesis, &zeroHash, ErrZeroExecutionBlockHash, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ProcessExecutionPayload(executionState(3, tt.parent), executionBlock(3, tt.payload))
			if tt.wantAs != nil {
				if !errors.As(err, tt.wantAs) {
					t.Fatalf("got %v, want %T", err, tt.wantAs)
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessExecutionPayloadCachesHeaderAndChains(t *testing.T) {
	state := executionState(1, [32]byte{0xE1})
	for slot := uint64(1); slot <= 2; slot++ {
		state.Slot = slot
		payload := &types.ExecutionPayload{
			ParentHash:   state.LatestExecutionPayloadHeader.BlockHash,
			Timestamp:    ComputeTimeAtSlot(testGenesisTime, slot),
			BlockHash:    [32]byte{byte(slot)},
			Transactions: [][]byte{{0x01}},
		}
		if err := ProcessExecutionPayload(state, executionBlock(slot, payload)); err != nil {
			t.Fatalf("slot %d: %v", slot, err)
		}
		want, err := payload.ToHeader()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(state.LatestExecutionPayloadHeader, *want) {
			t.Fatalf("slot %d: header was not cached from the applied payload", slot)
		}
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
