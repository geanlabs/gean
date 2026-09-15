package execution

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

// Run only against a disposable geth initialized with scripts/el-demo/genesis.json.
// GEAN_TEST_ENGINE_URL and GEAN_TEST_ENGINE_JWT must be explicitly supplied.
func TestGethExecutionFeaturePolicy(t *testing.T) {
	endpoint := os.Getenv("GEAN_TEST_ENGINE_URL")
	if endpoint == "" {
		t.Skip("requires an isolated geth Engine API")
	}
	secret, err := LoadJWTSecret(os.Getenv("GEAN_TEST_ENGINE_JWT"))
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(endpoint, secret)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	genesis, err := client.GenesisBlockHash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state := ForkchoiceState{HeadBlockHash: genesis, SafeBlockHash: genesis, FinalizedBlockHash: genesis}
	parentRoot := Hash{1}
	for _, unauthorized := range []bool{true, false} {
		attrs := &PayloadAttributes{Timestamp: Quantity(time.Now().Unix()), ParentBeaconBlockRoot: parentRoot, Withdrawals: []Withdrawal{}}
		if unauthorized {
			attrs.Withdrawals = []Withdrawal{{Address: Address{0x42}, Amount: 1_000_000_000}}
		}
		result, err := client.ForkchoiceUpdated(ctx, state, attrs)
		if err != nil || result.PayloadID == nil {
			t.Fatalf("prepare: %+v, %v", result, err)
		}
		var envelope getPayloadEnvelope
		if err := client.rpc.call(ctx, "engine_getPayloadV3", []any{*result.PayloadID}, &envelope); err != nil {
			t.Fatal(err)
		}
		payload := PayloadFromWire(envelope.ExecutionPayload)
		// Bypass gean's feature policy to show what the EL itself validates.
		var status PayloadStatus
		if err := client.rpc.call(ctx, "engine_newPayloadV3", []any{PayloadToWire(payload), []Hash{}, parentRoot}, &status); err != nil {
			t.Fatal(err)
		}
		if status.Status != StatusValid {
			t.Fatalf("geth verdict: %+v", status)
		}
		_, err = client.NewPayload(ctx, payload, parentRoot)
		if unauthorized {
			if !errors.Is(err, types.ErrExecutionWithdrawalsUnsupported) {
				t.Fatalf("withdrawal policy: %v", err)
			}
			t.Log("geth accepted an arbitrary withdrawal; gean rejected it")
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if payload.BlobGasUsed != 0 {
				t.Fatal("no-blob chain produced blob gas")
			}
			t.Log("empty-withdrawal, no-blob payload validated by geth and gean")
		}
	}
}
