package embedded

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/types"
)

// The demo genesis is the single description of the execution chain, shared
// by the remote Docker demo and these in-process tests.
const demoGenesis = "../../../scripts/el-demo/genesis.json"

func startTestEngine(t *testing.T) *Engine {
	t.Helper()
	genesis, err := LoadGenesis(filepath.FromSlash(demoGenesis))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Start(Config{Genesis: genesis})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func TestEmbeddedGenesisAndCapabilities(t *testing.T) {
	engine := startTestEngine(t)
	ctx := context.Background()

	genesis, _ := LoadGenesis(filepath.FromSlash(demoGenesis))
	hash, err := engine.GenesisBlockHash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if hash != [32]byte(genesis.ToBlock().Hash()) {
		t.Fatalf("genesis hash %x does not match the genesis file", hash)
	}

	supported, err := engine.ExchangeCapabilities(ctx, execution.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range execution.Capabilities {
		found := false
		for _, s := range supported {
			found = found || s == method
		}
		if !found {
			t.Fatalf("geth does not advertise %s: %v", method, supported)
		}
	}
}

func TestEmbeddedBuildAndImportCycle(t *testing.T) {
	engine := startTestEngine(t)
	ctx := context.Background()
	genesisHash, _ := engine.GenesisBlockHash(ctx)
	parentRoot := [32]byte{0xC0}
	state := execution.ForkchoiceState{HeadBlockHash: genesisHash, SafeBlockHash: genesisHash, FinalizedBlockHash: genesisHash}

	result, err := engine.ForkchoiceUpdated(ctx, state, execution.NewPayloadAttributes(uint64(time.Now().Unix()), [types.AddressSize]byte{0xAA}, parentRoot))
	if err != nil || result.PayloadStatus.Status != execution.StatusValid || result.PayloadID == nil {
		t.Fatalf("build request: %+v, %v", result, err)
	}

	payload, err := engine.GetPayload(ctx, *result.PayloadID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ParentHash != genesisHash || payload.BlockNumber != 1 || payload.FeeRecipient != [types.AddressSize]byte{0xAA} {
		t.Fatalf("built payload: %+v", payload)
	}

	// The same node executes its own build, then moves its head onto it.
	status, err := engine.NewPayload(ctx, payload, parentRoot)
	if err != nil || status.Status != execution.StatusValid {
		t.Fatalf("newPayload: %+v, %v", status, err)
	}
	state.HeadBlockHash = payload.BlockHash
	if result, err := engine.ForkchoiceUpdated(ctx, state, nil); err != nil || result.PayloadStatus.Status != execution.StatusValid {
		t.Fatalf("head update: %+v, %v", result, err)
	}
	if head := engine.backend.BlockChain().CurrentBlock(); head.Number.Uint64() != 1 || head.Hash() != payload.BlockHash {
		t.Fatalf("geth head did not follow: %v", head.Number)
	}

	// A wrong beacon root changes the block hash geth computes, so the same
	// payload is rejected rather than silently stored under another hash.
	status, err = engine.NewPayload(ctx, payload, [32]byte{0xC1})
	if err != nil || status.Status != execution.StatusInvalid {
		t.Fatalf("mismatched beacon root should be INVALID: %+v, %v", status, err)
	}
}
