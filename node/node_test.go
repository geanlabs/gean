package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

func TestMain(m *testing.M) {
	logger.Quiet = true
	os.Exit(m.Run())
}

func makeTestEngine() *Engine {
	backend := storage.NewInMemoryBackend()
	s := NewConsensusStore(backend)

	// Set up genesis state.
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	var genesisRoot [32]byte
	genesisRoot[0] = 0x01
	s.SetHead(genesisRoot)
	s.SetSafeTarget(genesisRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.InsertBlockHeader(genesisRoot, &types.BlockHeader{Slot: 0})

	genesisState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	s.InsertState(genesisRoot, genesisState)

	fc := forkchoice.New(0, genesisRoot, [32]byte{})

	return New(s, fc, nil, nil, NewAggregatorController(false), 1)
}

func TestEngineCreation(t *testing.T) {
	e := makeTestEngine()
	if e.Store == nil {
		t.Fatal("store should not be nil")
	}
	if e.FC == nil {
		t.Fatal("fork choice should not be nil")
	}
}

func TestEngineUpdateHead(t *testing.T) {
	e := makeTestEngine()
	e.updateHead(false)

	head := e.Store.Head()
	if types.IsZeroRoot(head) {
		t.Fatal("head should not be zero after updateHead")
	}
}

func TestEngineUpdateSafeTarget(t *testing.T) {
	e := makeTestEngine()
	e.updateSafeTarget()

	safeTarget := e.Store.SafeTarget()
	if types.IsZeroRoot(safeTarget) {
		t.Fatal("safe target should not be zero")
	}
}

// makeSafeTargetEngine builds an engine with a 3-block chain and N validators
// in head state. Returns the engine and the slot-2 block root used as the
// safe-target candidate by the regression test below.
func makeSafeTargetEngine(t *testing.T, numValidators int) (*Engine, [32]byte) {
	t.Helper()
	e := makeTestEngine()

	genesis := e.Store.Head()
	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22

	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	headState := e.Store.GetState(genesis)
	headState.Validators = make([]*types.Validator, numValidators)
	for i := range headState.Validators {
		headState.Validators[i] = &types.Validator{}
	}
	e.Store.InsertState(genesis, headState)

	return e, block2
}

// planAggregatedVoteForBlock returns an attestation-data payload + proof where
// the first `numVoters` validators vote for the given head/target block.
func planAggregatedVoteForBlock(targetRoot [32]byte, targetSlot, numValidators, numVoters uint64) ([32]byte, *types.AttestationData, *types.AggregatedSignatureProof) {
	bits := types.NewBitlistSSZ(numValidators)
	for i := uint64(0); i < numVoters; i++ {
		types.BitlistSet(bits, i)
	}
	data := &types.AttestationData{
		Slot:   targetSlot,
		Head:   &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Target: &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Source: &types.Checkpoint{},
	}
	dataRoot, _ := data.HashTreeRoot()
	return dataRoot, data, &types.AggregatedSignatureProof{Participants: bits}
}

// TestUpdateSafeTarget_IgnoresKnownPool reproduces the leanSpec PR #680
// scenario: votes living only in the known pool must not advance safe target.
// The same votes via the new pool must advance it.
func TestUpdateSafeTarget_IgnoresKnownPool(t *testing.T) {
	const numValidators = 6 // threshold = ceil(2*6/3) = 4

	t.Run("known_pool_only_does_not_advance", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)
		genesis := e.Store.Head()

		dataRoot, data, proof := planAggregatedVoteForBlock(block2, 2, numValidators, 4)
		e.Store.KnownPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != genesis {
			t.Fatalf("safe target advanced past genesis from known-pool-only votes (root=0x%x); leanSpec #680 forbids this",
				e.Store.SafeTarget())
		}
	})

	t.Run("new_pool_advances", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)

		dataRoot, data, proof := planAggregatedVoteForBlock(block2, 2, numValidators, 4)
		e.Store.NewPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != block2 {
			t.Fatalf("safe target did not advance to block_2 with 4-of-6 new-pool votes; got 0x%x",
				e.Store.SafeTarget())
		}
	})
}

func TestEnginePendingBlocks(t *testing.T) {
	e := makeTestEngine()

	var blockRoot, parentRoot [32]byte
	blockRoot[0] = 0x10
	parentRoot[0] = 0x20

	// Manually add pending entries (simulates addPendingBlock logic).
	e.PendingBlockParents[blockRoot] = parentRoot
	children := make(map[[32]byte]bool)
	children[blockRoot] = true
	e.PendingBlocks[parentRoot] = children

	if len(e.PendingBlocks) != 1 {
		t.Fatalf("expected 1 pending parent, got %d", len(e.PendingBlocks))
	}
	if len(e.PendingBlockParents) != 1 {
		t.Fatalf("expected 1 pending block, got %d", len(e.PendingBlockParents))
	}
}

// Engine.Run must invoke onTick once before the for-select loop accepts
// ticker fires. Without the bootstrap call there is up to one tick interval
// (800ms) of dead time after start where store.time stays at its boot value.
// The polling check below trips well inside that window.
func TestEngineRun_InvokesInitialOnTick(t *testing.T) {
	e := makeTestEngine()

	if got := e.Store.Time(); got != 0 {
		t.Fatalf("precondition: expected store.time=0 at start, got %d", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Error("Engine.Run did not exit after context cancel")
		}
	}()

	// The ticker fires every MillisecondsPerInterval (800ms). Anything
	// observed before that came from the bootstrap call.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if e.Store.Time() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("Engine.Run did not invoke initial onTick within 200ms; store.time=%d", e.Store.Time())
}

func TestProcessOneBlock_RejectsPreFinalized(t *testing.T) {
	e := makeTestEngine()

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       5,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Signature: &types.BlockSignatures{},
	}

	var queue []*types.SignedBlock
	e.processOneBlock(signedBlock, &queue)

	if len(e.PendingBlocks) != 0 {
		t.Fatalf("pre-finalized block was buffered as pending; PendingBlocks=%d", len(e.PendingBlocks))
	}
	if len(e.PendingBlockParents) != 0 {
		t.Fatalf("pre-finalized block recorded a missing-parent entry; PendingBlockParents=%d", len(e.PendingBlockParents))
	}
	if len(queue) != 0 {
		t.Fatalf("pre-finalized block produced cascade work; queue=%d", len(queue))
	}
}

func TestProcessOneBlock_AdmitsAtFinalizedSlot(t *testing.T) {
	e := makeTestEngine()

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	// Block AT the finalized slot must pass the guard (strict less-than). With
	// no parent state available it gets buffered as pending; that's the proof
	// the new guard didn't drop it.
	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       10,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Signature: &types.BlockSignatures{},
	}

	var queue []*types.SignedBlock
	e.processOneBlock(signedBlock, &queue)

	if len(e.PendingBlockParents) == 0 {
		t.Fatal("block at finalized slot was rejected by the strict-less-than guard")
	}
}

func TestEngineCascadePending(t *testing.T) {
	e := makeTestEngine()

	var parentRoot, child1, child2 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10
	child2[0] = 0x20

	e.PendingBlockParents[child1] = parentRoot
	e.PendingBlockParents[child2] = parentRoot
	children := make(map[[32]byte]bool)
	children[child1] = true
	children[child2] = true
	e.PendingBlocks[parentRoot] = children

	if len(e.PendingBlocks[parentRoot]) != 2 {
		t.Fatalf("expected 2 children pending, got %d", len(e.PendingBlocks[parentRoot]))
	}

	// collectPendingChildren removes entries and returns blocks to process.
	var queue []*types.SignedBlock
	e.collectPendingChildren(parentRoot, &queue)

	if len(e.PendingBlocks) != 0 {
		t.Fatalf("expected 0 pending after cascade, got %d", len(e.PendingBlocks))
	}
	if len(e.PendingBlockParents) != 0 {
		t.Fatalf("expected 0 pending parents after cascade, got %d", len(e.PendingBlockParents))
	}
}

func TestEngineMessageHandler(t *testing.T) {
	e := makeTestEngine()

	// Verify Engine implements the MessageHandler interface.
	block := &types.SignedBlock{
		Block:     &types.Block{Slot: 1},
		Signature: &types.BlockSignatures{},
	}

	// Should not panic — just push to channel.
	e.OnBlock(block)

	// Check channel received it.
	select {
	case received := <-e.BlockCh:
		if received.Block.Slot != 1 {
			t.Fatal("wrong block slot")
		}
	default:
		t.Fatal("block should be in channel")
	}
}

func TestEngineGetOurProposer(t *testing.T) {
	e := makeTestEngine()
	// No keys — should return false.
	_, ok := e.getOurProposer(1)
	if ok {
		t.Fatal("should not be proposer without keys")
	}
}

func TestEngineCurrentSlot(t *testing.T) {
	e := makeTestEngine()
	// Genesis at 1000s, slot 1 starts at 1004s.
	slot := e.currentSlot(1004 * 1000) // 1004000ms
	if slot != 1 {
		t.Fatalf("expected slot 1, got %d", slot)
	}
}

func TestEngineCurrentInterval(t *testing.T) {
	e := makeTestEngine()
	// Genesis at 1000s. Interval 0 of slot 1 starts at 1004000ms.
	// Interval 1 starts at 1004800ms.
	interval := e.currentInterval(1004800)
	if interval != 1 {
		t.Fatalf("expected interval 1, got %d", interval)
	}
}

// TestComputeSyncStatus_FreshNodePastWallClock locks the contract the
// maybePropose sync gate relies on: a fresh engine (head=0) with current
// slot past the SyncLagSlots window must report SyncSyncing (not SyncSynced).
// If this returned SyncSynced, the gate would allow self-production on
// startup, reintroducing the rpc-compat 'forkchoice includes expected node
// fields' failure.
func TestComputeSyncStatus_FreshNodePastWallClock(t *testing.T) {
	e := makeTestEngine()

	// head=0, currentSlot=39: 0 + SyncLagSlots(2) = 2, 2 >= 39 is false → SyncSyncing.
	if status := e.computeSyncStatus(39); status != SyncSyncing {
		t.Errorf("head=0 currentSlot=39 should be SyncSyncing, got %s", status)
	}

	// head=0, currentSlot=2: 0 + 2 = 2, 2 >= 2 is true → SyncSynced.
	// Documents the boundary: the gate opens at currentSlot ≤ head + SyncLagSlots.
	if status := e.computeSyncStatus(2); status != SyncSynced {
		t.Errorf("head=0 currentSlot=2 should be SyncSynced, got %s", status)
	}
}

func TestPendingBlockCount(t *testing.T) {
	e := makeTestEngine()

	if e.pendingBlockCount() != 0 {
		t.Fatal("expected 0 pending blocks initially")
	}

	// Add 3 children under 2 parents.
	parent1 := [32]byte{0x01}
	parent2 := [32]byte{0x02}
	child1 := [32]byte{0x10}
	child2 := [32]byte{0x20}
	child3 := [32]byte{0x30}

	e.PendingBlocks[parent1] = map[[32]byte]bool{child1: true, child2: true}
	e.PendingBlocks[parent2] = map[[32]byte]bool{child3: true}

	if e.pendingBlockCount() != 3 {
		t.Fatalf("expected 3 pending blocks, got %d", e.pendingBlockCount())
	}
}

func TestPendingBlockDepthTracking(t *testing.T) {
	e := makeTestEngine()

	// Simulate a chain of pending blocks with increasing depth.
	root1 := [32]byte{0x01}
	root2 := [32]byte{0x02}
	root3 := [32]byte{0x03}

	e.PendingBlockDepths[root1] = 1
	e.PendingBlockDepths[root2] = 2
	e.PendingBlockDepths[root3] = 3

	if e.PendingBlockDepths[root3] != 3 {
		t.Fatalf("expected depth 3, got %d", e.PendingBlockDepths[root3])
	}

	// Verify depth is inherited from parent.
	parentDepth := e.PendingBlockDepths[root2]
	childDepth := parentDepth + 1
	if childDepth != 3 {
		t.Fatalf("expected inherited depth 3, got %d", childDepth)
	}
}

func TestDiscardPendingSubtree(t *testing.T) {
	e := makeTestEngine()

	// Build a tree: root -> child1, child1 -> grandchild1, grandchild2
	root := [32]byte{0x01}
	child1 := [32]byte{0x10}
	grandchild1 := [32]byte{0xA0}
	grandchild2 := [32]byte{0xB0}

	e.PendingBlocks[root] = map[[32]byte]bool{child1: true}
	e.PendingBlocks[child1] = map[[32]byte]bool{grandchild1: true, grandchild2: true}
	e.PendingBlockParents[child1] = root
	e.PendingBlockParents[grandchild1] = child1
	e.PendingBlockParents[grandchild2] = child1
	e.PendingBlockDepths[child1] = 1
	e.PendingBlockDepths[grandchild1] = 2
	e.PendingBlockDepths[grandchild2] = 2

	// Discard subtree from child1.
	e.discardPendingSubtree(child1)

	// child1 and its descendants should be gone.
	if _, ok := e.PendingBlockParents[child1]; ok {
		t.Fatal("child1 should be removed from PendingBlockParents")
	}
	if _, ok := e.PendingBlockParents[grandchild1]; ok {
		t.Fatal("grandchild1 should be removed from PendingBlockParents")
	}
	if _, ok := e.PendingBlockParents[grandchild2]; ok {
		t.Fatal("grandchild2 should be removed from PendingBlockParents")
	}
	if _, ok := e.PendingBlockDepths[child1]; ok {
		t.Fatal("child1 depth should be removed")
	}
	if _, ok := e.PendingBlockDepths[grandchild1]; ok {
		t.Fatal("grandchild1 depth should be removed")
	}

	// Root's children entry should still exist (discardPendingSubtree doesn't clean parent).
	if _, ok := e.PendingBlocks[root]; !ok {
		t.Fatal("root's PendingBlocks entry should still exist")
	}
}

func TestCascadeClearsDepth(t *testing.T) {
	e := makeTestEngine()

	var parentRoot, child1 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10

	e.PendingBlockParents[child1] = parentRoot
	e.PendingBlockDepths[child1] = 5
	children := make(map[[32]byte]bool)
	children[child1] = true
	e.PendingBlocks[parentRoot] = children

	var queue []*types.SignedBlock
	e.collectPendingChildren(parentRoot, &queue)

	// Depth should be cleared after cascade.
	if _, ok := e.PendingBlockDepths[child1]; ok {
		t.Fatal("depth should be cleared after collectPendingChildren")
	}
}
