package node

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/execution/embedded"
	"github.com/geanlabs/gean/internal/types"
)

// The driver's whole proposal and import path against a real geth running in
// this process: request a build, collect it, submit it, verify it as an
// incoming block, and see the forkchoice update land on an executed block.
func TestExecutionDriverAgainstEmbeddedGeth(t *testing.T) {
	genesis, err := embedded.LoadGenesis(filepath.FromSlash("../../scripts/el-demo/genesis.json"))
	if err != nil {
		t.Fatal(err)
	}
	geth, err := embedded.Start(embedded.Config{Genesis: genesis})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = geth.Close() })

	e := makeTestEngine()
	e.Execution = NewExecutionDriver(geth, e.Store, [types.AddressSize]byte{0xAA})
	ctx := context.Background()

	// Anchor the consensus head on a block whose payload is the execution
	// genesis, as a real genesis body would be.
	elGenesis, _ := geth.GenesisBlockHash(ctx)
	head := [32]byte{0xB0}
	storeBlockWithPayloadHash(e, head, 0, [32]byte{}, elGenesis)
	e.Store.SetHead(head)
	e.Execution.remember(head, elGenesis)

	// Slot 1's wall-clock time must be later than the execution genesis.
	genesisTime := uint64(time.Now().Unix()) - types.SecondsPerSlot
	e.Execution.prepare(1, head, e.Execution.forkchoiceState(), genesisTime)
	waitFor(t, "prepared build", func() bool {
		e.Execution.mu.Lock()
		defer e.Execution.mu.Unlock()
		return e.Execution.prepared != nil
	})

	payload, reason := e.Execution.takePayload(ctx, 1, head)
	if payload == nil {
		t.Fatalf("takePayload: %s", reason)
	}
	if payload.ParentHash != elGenesis || payload.BlockNumber != 1 {
		t.Fatalf("payload does not extend the execution genesis: %+v", payload)
	}
	if !e.Execution.submit(ctx, payload, head) {
		t.Fatal("geth did not accept our own payload")
	}

	// A peer receiving the block runs the same payload through the verifier.
	block := &types.SignedBlock{Block: &types.Block{Slot: 1, ParentRoot: head, Body: &types.BlockBody{ExecutionPayload: *payload}}}
	if verdict := e.Execution.checkPayload(ctx, block); verdict != executionValid {
		t.Fatalf("verifier verdict %v, want valid", verdict)
	}

	// Once imported, the head-change forkchoice update lands on an executed
	// block and comes back VALID.
	blockRoot := [32]byte{0xB1}
	e.Execution.remember(blockRoot, payload.BlockHash)
	e.Store.SetHead(blockRoot)
	result, err := e.Execution.forkchoiceUpdated(ctx, e.Execution.forkchoiceState(), nil)
	if err != nil || result.PayloadStatus.Status != "VALID" {
		t.Fatalf("forkchoice update: %+v, %v", result, err)
	}
	if !e.Execution.headValidated() {
		t.Fatal("head should be recorded as execution-valid")
	}
}
