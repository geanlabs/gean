package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/dutygate"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/shadow"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/syncer"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

func makeTestEngine() *Engine {
	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

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

	return New(clock.Real{}, s, fc, nil, nil, role.New(false), 1, shadow.Rates{})
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
	e.updateHead()

	head := e.Store.Head()
	if types.IsZeroRoot(head) {
		t.Fatal("head should not be zero after updateHead")
	}
}

// stateFinalizing builds an SSZ-serializable post-state pinned to the given
// finalized checkpoint, so it round-trips through the store like a real state.
func stateFinalizing(slot uint64, finalized *types.Checkpoint) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          finalized,
		LatestFinalized:          finalized,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestUpdateFinalizedFollowsCanonicalHead(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22

	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	// The head's post-state finalizes block1; finalization must re-anchor there.
	e.Store.InsertState(block2, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))
	e.Store.SetHead(block2)

	e.updateFinalizedFromHead(block2)

	got := e.Store.LatestFinalized()
	if got == nil || got.Slot != 1 || got.Root != block1 {
		t.Fatalf("finalized=%v, want slot 1 root 0x%x", got, block1)
	}
}

func TestDeriveFinalizedKeepsAnchorWhenNoBlockAtSlot(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1 [32]byte
	block1[0] = 0x11
	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 2, ParentRoot: genesis})
	// Post-state claims a finalized slot with no block on the chain at that slot
	// (e.g. a checkpoint-sync anchor); the trusted checkpoint must stand.
	e.Store.InsertState(block1, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))

	if cp := store.DeriveFinalizedFromHead(e.Store, block1); cp != nil {
		t.Fatalf("expected nil to keep anchor, got slot %d root 0x%x", cp.Slot, cp.Root)
	}
}

func TestUpdateFinalizedDoesNotLatchAboveHead(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22
	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	// A losing fork briefly latched finalization at slot 2; the canonical head's
	// post-state only finalizes block1 at slot 1, so finalization must move down
	// to track the head rather than stay latched above it.
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: block2, Slot: 2})
	e.Store.InsertState(block2, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))
	e.Store.SetHead(block2)

	e.updateFinalizedFromHead(block2)

	got := e.Store.LatestFinalized()
	if got == nil || got.Slot != 1 || got.Root != block1 {
		t.Fatalf("finalized=%v, want slot 1 root 0x%x (must not latch above head)", got, block1)
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

func planAggregatedVoteForBlock(t *testing.T, targetRoot [32]byte, targetSlot, numValidators, numVoters uint64) ([32]byte, *types.AttestationData, *types.SingleMessageAggregate) {
	t.Helper()

	bits := types.NewBitlistSSZ(numValidators)
	for i := range numVoters {
		types.BitlistSet(bits, i)
	}
	data := &types.AttestationData{
		Slot:   targetSlot,
		Head:   &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Target: &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Source: &types.Checkpoint{},
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	return dataRoot, data, &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}}
}

func TestUpdateSafeTarget_IgnoresKnownPool(t *testing.T) {
	const numValidators = 6

	t.Run("known_pool_only_does_not_advance", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)
		genesis := e.Store.Head()

		dataRoot, data, proof := planAggregatedVoteForBlock(t, block2, 2, numValidators, 4)
		e.Store.KnownPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != genesis {
			t.Fatalf("safe target advanced past genesis from known-pool-only votes (root=0x%x); safe target must ignore known-pool-only votes",
				e.Store.SafeTarget())
		}
	})

	t.Run("new_pool_advances", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)

		dataRoot, data, proof := planAggregatedVoteForBlock(t, block2, 2, numValidators, 4)
		e.Store.NewPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != block2 {
			t.Fatalf("safe target did not advance to block_2 with 4-of-6 new-pool votes; got 0x%x",
				e.Store.SafeTarget())
		}
	})

	t.Run("promoted_votes_stop_counting", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)
		genesis := e.Store.Head()

		dataRoot, data, proof := planAggregatedVoteForBlock(t, block2, 2, numValidators, 4)
		e.Store.NewPayloads.Push(dataRoot, data, proof)
		e.updateSafeTarget()
		if e.Store.SafeTarget() != block2 {
			t.Fatalf("precondition: 4-of-6 new-pool votes should reach block_2; got 0x%x", e.Store.SafeTarget())
		}

		// The next slot's pool holds one vote; the four promoted votes must not count.
		e.Store.PromoteNewToKnown()
		dataRoot, data, proof = planAggregatedVoteForBlock(t, block2, 2, numValidators, 1)
		e.Store.NewPayloads.Push(dataRoot, data, proof)
		e.updateSafeTarget()

		if e.Store.SafeTarget() != genesis {
			t.Fatalf("safe target counted promoted votes: got 0x%x, want genesis", e.Store.SafeTarget())
		}
	})
}

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

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if e.Store.Time() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("Engine.Run did not invoke initial onTick within 200ms; store.time=%d", e.Store.Time())
}

func TestOnTickNilAggregationController(t *testing.T) {
	e := makeTestEngine()
	e.AggCtl = nil

	e.onTick()
}

func TestEngineMessageHandler(t *testing.T) {
	e := makeTestEngine()

	block := &types.SignedBlock{
		Block: &types.Block{Slot: 1},
		Proof: &types.MultiMessageAggregate{},
	}

	e.OnBlock(block)

	select {
	case received := <-e.Importer.Blocks():
		if received.Block.Slot != 1 {
			t.Fatal("wrong block slot")
		}
	default:
		t.Fatal("block should be in channel")
	}
}

func TestEngineGetOurProposer(t *testing.T) {
	e := makeTestEngine()
	_, ok := e.getOurProposer(1)
	if ok {
		t.Fatal("should not be proposer without keys")
	}
}

func TestEngineCurrentSlot(t *testing.T) {
	e := makeTestEngine()
	slot := e.currentSlot(1004000)
	if slot != 1 {
		t.Fatalf("expected slot 1, got %d", slot)
	}
}

func TestEngineCurrentInterval(t *testing.T) {
	e := makeTestEngine()
	interval := e.currentInterval(1004800)
	if interval != 1 {
		t.Fatalf("expected interval 1, got %d", interval)
	}
}

func TestEngineClockHandlesOverflowGenesisTime(t *testing.T) {
	e := makeTestEngine()
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: ^uint64(0)/1000 + 1})

	if slot := e.currentSlot(^uint64(0)); slot != 0 {
		t.Fatalf("overflow genesis currentSlot=%d, want 0", slot)
	}
	if interval := e.currentInterval(^uint64(0)); interval != 0 {
		t.Fatalf("overflow genesis currentInterval=%d, want 0", interval)
	}
}

func TestNilEngineClockReturnsZero(t *testing.T) {
	var e *Engine
	if slot := e.currentSlot(1); slot != 0 {
		t.Fatalf("nil engine currentSlot=%d, want 0", slot)
	}
	if interval := e.currentInterval(1); interval != 0 {
		t.Fatalf("nil engine currentInterval=%d, want 0", interval)
	}
}

func TestComputeSyncStatus_FreshNodePastWallClock(t *testing.T) {
	e := makeTestEngine()

	if status := e.computeSyncStatus(39); status != syncer.SyncSyncing {
		t.Errorf("head=0 currentSlot=39 should be syncer.SyncSyncing, got %s", status)
	}

	if status := e.computeSyncStatus(2); status != syncer.SyncSynced {
		t.Errorf("head=0 currentSlot=2 should be syncer.SyncSynced, got %s", status)
	}
}

func TestComputeSyncStatusAvoidsHeadSlotOverflow(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	e.Store.InsertBlockHeader(head, &types.BlockHeader{Slot: ^uint64(0)})

	if status := e.computeSyncStatus(^uint64(0)); status != syncer.SyncSynced {
		t.Fatalf("max head/current slot status=%s, want %s", status, syncer.SyncSynced)
	}
}

// networkSeenSlot must reflect gossip the node heard but could not import, so a
// node whose own chain is stalled doesn't mistake its stall for the network's.
func TestNetworkSeenSlotPrefersGossipWhenAheadOfStored(t *testing.T) {
	e := makeTestEngine()
	// Genesis ~100 slots ago so realistic gossip slots sit inside the horizon.
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: uint64(time.Now().Unix()) - 400})

	stored := e.Store.MaxStoredBlockSlot()
	e.noteGossipSlot(&types.SignedBlock{Block: &types.Block{Slot: stored + 50}})

	if got := e.networkSeenSlot(); got != stored+50 {
		t.Fatalf("networkSeenSlot=%d, want gossip-seen %d", got, stored+50)
	}
}

// A far-future gossip slot must not move the network-seen marker; otherwise a
// hostile peer could pin the duty gate's network-stall carve-out shut.
func TestNoteGossipSlotIgnoresFarFuture(t *testing.T) {
	e := makeTestEngine()
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: uint64(time.Now().Unix()) - 400})

	e.noteGossipSlot(&types.SignedBlock{Block: &types.Block{Slot: 50}})
	e.noteGossipSlot(&types.SignedBlock{Block: &types.Block{Slot: 1_000_000}})

	if got := e.maxSeenGossipSlot.Load(); got != 50 {
		t.Fatalf("far-future gossip slot should be ignored, max=%d want 50", got)
	}
}

// Regression: a marooned node (own fork advancing with wall clock, live network
// far ahead on gossip) must stay gated off duties. Feeding the duty gate stored
// fork slots triggered the network-stall carve-out and kept it proposing on the
// dead fork; feeding it the gossip-seen slot keeps the gate shut.
func TestDutyGateClosedForMaroonedNodeViaGossipSeenSlot(t *testing.T) {
	const wallSlot, forkHead, forkStored, gossipSeen = 64185, 63663, 63663, 64185

	stale := dutygate.New()
	if !stale.Decide("block", wallSlot, forkHead, forkStored) {
		t.Fatal("precondition: stored-slot wiring wrongly reopens via network-stall carve-out")
	}

	fixed := dutygate.New()
	if fixed.Decide("block", wallSlot, forkHead, gossipSeen) {
		t.Fatal("marooned node should be gated off its dead fork with gossip-seen slot")
	}
}
