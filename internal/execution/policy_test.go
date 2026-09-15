package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestClientRejectsUnsupportedPayloads(t *testing.T) {
	for name, payload := range map[string]*types.ExecutionPayload{
		"withdrawal":       {Withdrawals: []*types.Withdrawal{{Amount: 1}}},
		"blob transaction": {Transactions: [][]byte{{0x03, 0xc0}}},
		"blob gas":         {BlobGasUsed: 131072},
	} {
		t.Run(name, func(t *testing.T) {
			client, fake := testClient(t)
			if _, err := client.NewPayload(context.Background(), payload, [32]byte{}); err == nil {
				t.Fatal("unsupported payload was submitted")
			}
			if len(fake.seen) != 0 {
				t.Fatal("unsupported payload reached Engine API")
			}
			encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": getPayloadEnvelope{ExecutionPayload: PayloadToWire(payload)}})
			if err != nil {
				t.Fatal(err)
			}
			fake.replies["engine_getPayloadV3"] = string(encoded)
			if got, err := client.GetPayload(context.Background(), PayloadID{}); err == nil || got != nil {
				t.Fatal("unsupported builder response was accepted")
			}
		})
	}
}
