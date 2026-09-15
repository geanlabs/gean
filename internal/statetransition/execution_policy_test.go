package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestExecutionRejectsUnsupportedOperations(t *testing.T) {
	for _, name := range []string{"withdrawal", "nil withdrawal", "blob transaction", "blob gas"} {
		t.Run(name, func(t *testing.T) {
			state := executionState(1, [32]byte{1})
			before := state.LatestExecutionPayloadHeader
			payload := &types.ExecutionPayload{ParentHash: before.BlockHash, BlockHash: [32]byte{2}, Timestamp: ComputeTimeAtSlot(testGenesisTime, 1)}
			switch name {
			case "withdrawal":
				payload.Withdrawals = []*types.Withdrawal{{Address: [20]byte{1}, Amount: 1_000_000_000}}
			case "nil withdrawal":
				payload.Withdrawals = []*types.Withdrawal{nil}
			case "blob transaction":
				payload.Transactions = [][]byte{{0x03, 0xc0}}
			case "blob gas":
				payload.BlobGasUsed = 131072
			}
			if err := ProcessExecutionPayload(state, executionBlock(1, payload)); err == nil {
				t.Fatal("unsupported operation was accepted")
			}
			if state.LatestExecutionPayloadHeader.BlockHash != before.BlockHash {
				t.Fatal("rejected payload advanced the execution header")
			}
		})
	}
}
