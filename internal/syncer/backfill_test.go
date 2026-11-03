package syncer

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

func TestSyncDriver_CheckAndBackfill_PeerNotAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 0}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls when peer not ahead, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_GapBelowThreshold(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: blocksByRangeSyncThreshold}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls for gap below threshold, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_FetchesWhenAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1, 2)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 2 {
		t.Errorf("expected 2 range calls (drain + empty probe), got %d", got)
	}

	got := drainBlockCh(t, n, 2, 100*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_LoopsUntilCaughtUp(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1, 2, 3, 4)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks[:2],
			blocks[2:],
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 3 {
		t.Errorf("expected 3 range calls (2 batches + empty probe), got %d", got)
	}

	got := drainBlockCh(t, n, 4, 100*time.Millisecond)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_CapsRangeCount(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: types.MaxRequestBlocks + 100})

	mock.mu.Lock()
	got := mock.rangeCounts[0]
	mock.mu.Unlock()
	if got != types.MaxRequestBlocks {
		t.Fatalf("range count=%d, want %d", got, types.MaxRequestBlocks)
	}
}

// A full batch of skipped slots must not end the pass: every later pass would
// ask for the same empty batch and never reach the blocks behind it.
func TestSyncDriver_CheckAndBackfill_AdvancesPastEmptyRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	first := uint64(types.MaxRequestBlocks + 1)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			nil,
			makeSyncRange(store.Head(), first, first+1),
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: first + 1})

	if want := []uint64{1, first}; !slices.Equal(mock.rangeStarts, want) {
		t.Fatalf("range starts=%v, want %v", mock.rangeStarts, want)
	}
	if got := drainBlockCh(t, n, 2, 100*time.Millisecond); len(got) != 2 {
		t.Fatalf("expected 2 blocks fed to engine, got %d", len(got))
	}
}

// A fallback peer never claimed the range, so its empty answer says nothing
// about whether the slots were skipped.
func TestSyncDriver_CheckAndBackfill_StopsOnEmptyFallbackRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeErr:     fmt.Errorf("simulated range fetch failure"),
		retryBatches: [][]*types.SignedBlock{nil},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: types.MaxRequestBlocks + 100})

	if got := mock.retryCalls.Load(); got != 1 {
		t.Fatalf("retry calls=%d, want 1", got)
	}
}

// A peer claiming a far head during a long finality stall must not get a request
// per batch of the stall by answering empty. A delivered block restarts the count.
func TestSyncDriver_CheckAndBackfill_EndsPassAfterEmptyBatches(t *testing.T) {
	n, st := makeTestSyncHarness()
	headSlot := uint64(100_000)
	header := &types.BlockHeader{Slot: headSlot}
	root := mustHeaderRoot(header)
	st.InsertBlockHeader(root, header)
	st.SetHead(root)
	st.SetLatestFinalized(&types.Checkpoint{Slot: 0})
	blockSlot := uint64(maxEmptyRangeBatches-1)*types.MaxRequestBlocks + 1
	batches := make([][]*types.SignedBlock, maxEmptyRangeBatches-1)
	batches = append(batches, makeSyncRange(root, blockSlot))
	mock := &mockSyncP2P{rangeBatches: batches}
	sd := NewSyncDriver(context.Background(), n, st, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: headSlot, FinalizedSlot: 100})

	if got, want := mock.rangeCalls.Load(), int32(2*maxEmptyRangeBatches); got != want {
		t.Fatalf("range calls=%d, want %d", got, want)
	}
}

// Every peer ahead of our finalized slot would otherwise fetch the same range.
func TestSyncDriver_CheckAndBackfill_RunsOneMaroonedPass(t *testing.T) {
	n, st := makeTestSyncHarness()
	st.SetLatestFinalized(&types.Checkpoint{Slot: 0})
	status := &p2p.StatusMessage{HeadSlot: 1, FinalizedSlot: forkReconcileFinalizedThreshold + 1}
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, st, mock)
	// p2 polls while p1's request is in flight.
	mock.onRange = func() {
		mock.onRange = nil
		sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p2"), status)
	}

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), status)
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Fatalf("range calls with a pass running=%d, want 1", got)
	}

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p2"), status)
	if got := mock.rangeCalls.Load(); got != 2 {
		t.Fatalf("range calls after the pass ended=%d, want 2", got)
	}
}

func TestSyncDriver_CheckAndBackfill_StopsOnMalformedRangeBlock(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{nil},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("malformed block should not reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsBlockBeforeRequestedSlot(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{{Block: &types.Block{Slot: 0, Body: &types.BlockBody{}}}},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("old range block should not reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsNonMonotonicRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
			},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("non-monotonic range should not partially reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsDisconnectedRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1)
	badBlock, _ := makeSyncBlock(2, [32]byte{0xff})
	blocks = append(blocks, badBlock)
	mock := &mockSyncP2P{rangeBatches: [][]*types.SignedBlock{blocks}}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("disconnected range should not partially reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_FallsBackToRoot(t *testing.T) {
	n, store := makeTestSyncHarness()
	rootBlock, _ := makeSyncBlock(100, types.ZeroRoot)
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		retryErr: fmt.Errorf("simulated retry exhaustion"),
		rootBlocks: []*types.SignedBlock{
			rootBlock,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	var headRoot [32]byte
	headRoot[0] = 0xAB
	peerStatus := &p2p.StatusMessage{HeadSlot: 100, HeadRoot: headRoot}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call (failed), got %d", got)
	}
	if got := mock.retryCalls.Load(); got != 1 {
		t.Errorf("expected 1 retry on other peers (exhausted), got %d", got)
	}
	if got := mock.rootCalls.Load(); got != 1 {
		t.Errorf("expected 1 root fallback call, got %d", got)
	}

	got := drainBlockCh(t, n, 1, 100*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 block from root fallback, got %d", len(got))
	}
}

// A failed range request must not end the pass: the batch, and every batch
// after it, goes to other peers. Every peer that failed stays excluded for the
// rest of the pass, so a stalled peer is not asked again for each batch.
func TestSyncDriver_CheckAndBackfill_RetriesFailedRangeOnOtherPeers(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1, 2, 3, 4)
	mock := &mockSyncP2P{
		rangeErr:     fmt.Errorf("simulated range fetch failure"),
		retryBatches: [][]*types.SignedBlock{blocks[:2], blocks[2:]},
		retryFails:   []libp2ppeer.ID{"p2"}, // p2 stalls during the first retry
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 4})

	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("failed peer asked %d times, want 1", got)
	}
	want := [][]libp2ppeer.ID{{"p1"}, {"p1", "p2"}}
	if !reflect.DeepEqual(mock.retryExcluded, want) {
		t.Errorf("retries excluded %v, want %v", mock.retryExcluded, want)
	}
	if got := mock.rootCalls.Load(); got != 0 {
		t.Errorf("head-by-root fallback ran %d times, want 0", got)
	}
	if got := drainBlockCh(t, n, 4, 100*time.Millisecond); len(got) != 4 {
		t.Fatalf("delivered %d blocks, want 4", len(got))
	}
}

// Shutdown fails the in-flight request; that is not the peer's fault, and
// retrying or falling back against a cancelled context only logs blame.
func TestSyncDriver_CheckAndBackfill_ShutdownDoesNotRetry(t *testing.T) {
	n, store := makeTestSyncHarness()
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockSyncP2P{rangeErr: context.Canceled, onRange: cancel}
	sd := NewSyncDriver(ctx, n, store, mock)

	sd.checkAndBackfill(ctx, libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := mock.retryCalls.Load(); got != 0 {
		t.Errorf("retries after shutdown=%d, want 0", got)
	}
	if got := mock.rootCalls.Load(); got != 0 {
		t.Errorf("head-by-root fallback after shutdown=%d, want 0", got)
	}
}

// Shutdown during the retry on other peers: the retry fails because the context
// ended, not because every peer did, so the head-by-root fallback must not run.
func TestSyncDriver_CheckAndBackfill_ShutdownDuringRetryDoesNotFallBack(t *testing.T) {
	n, store := makeTestSyncHarness()
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		retryErr: context.Canceled,
		onRetry:  cancel,
	}
	sd := NewSyncDriver(ctx, n, store, mock)

	sd.checkAndBackfill(ctx, libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := mock.retryCalls.Load(); got != 1 {
		t.Errorf("retries=%d, want 1", got)
	}
	if got := mock.rootCalls.Load(); got != 0 {
		t.Errorf("head-by-root fallback after shutdown=%d, want 0", got)
	}
}

func TestSyncDriver_CheckAndBackfill_FallbackSkipsMalformedBlocks(t *testing.T) {
	n, store := makeTestSyncHarness()
	rootBlock, _ := makeSyncBlock(100, types.ZeroRoot)
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		retryErr: fmt.Errorf("simulated retry exhaustion"),
		rootBlocks: []*types.SignedBlock{
			nil,
			{},
			{Block: &types.Block{Slot: 99}},
			rootBlock,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	got := drainBlockCh(t, n, 1, 100*time.Millisecond)
	if len(got) != 1 || got[0].Block.Slot != 100 {
		t.Fatalf("fallback forwarded %d blocks, want only valid slot 100", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_PerPeerDedup(t *testing.T) {
	n, store := makeTestSyncHarness()
	rangeBlock := make(chan struct{})
	blocks := makeSyncRange(store.Head(), 1)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks,
		},
		rangeBlock: rangeBlock,
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	peerID := libp2ppeer.ID("p1")

	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	}()

	for mock.rangeCalls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}

	sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call after dedup, got %d", got)
	}

	close(rangeBlock)
	<-done1
	_ = drainBlockCh(t, n, 1, 100*time.Millisecond)
}

// A full engine channel must stall the backfill, not drop blocks: dropped
// blocks are never re-requested (the range cursor advances past them), which
// permanently disconnects the chain.
func TestSyncDriver_CheckAndBackfill_BackpressuresInsteadOfDropping(t *testing.T) {
	n, store := makeTestSyncHarness()
	n.BlockCh = make(chan *types.SignedBlock, 1)
	blocks := makeSyncRange(store.Head(), 1, 2, 3, 4)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{blocks},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})
	}()

	got := drainBlockCh(t, n, 4, time.Second)
	if len(got) != 4 {
		t.Fatalf("expected all 4 blocks delivered despite capacity-1 channel, got %d", len(got))
	}
	<-done
}

// Cancellation while delivery is stalled must abort the backfill cleanly
// rather than advance to the next range.
func TestSyncDriver_CheckAndBackfill_CancelAbortsStalledDelivery(t *testing.T) {
	n, store := makeTestSyncHarness()
	n.BlockCh = make(chan *types.SignedBlock, 1)
	blocks := makeSyncRange(store.Head(), 1, 2, 3)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{blocks, blocks},
	}
	ctx, cancel := context.WithCancel(context.Background())
	sd := NewSyncDriver(ctx, n, store, mock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.checkAndBackfill(ctx, libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})
	}()

	// One block fits the channel; the feed is now stalled on the second.
	_ = drainBlockCh(t, n, 1, time.Second)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backfill did not abort after cancellation")
	}
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected no further range fetches after aborted delivery, got %d calls", got)
	}
}

func TestSyncDriver_ShouldBackfill_MaroonedOnFork(t *testing.T) {
	n, st := makeTestSyncHarness()
	// Our head kept pace at slot 100, but finalized is stuck at genesis: the
	// marooned-on-own-fork signature that by-root fetches cannot recover.
	head := &types.BlockHeader{Slot: 100}
	root := mustHeaderRoot(head)
	st.SetHead(root)
	st.InsertBlockHeader(root, head)
	st.SetLatestFinalized(&types.Checkpoint{Slot: 0})
	sd := NewSyncDriver(context.Background(), n, st, &mockSyncP2P{})

	// Peer at the same head but finalized far ahead -> reconcile.
	if !sd.shouldBackfill(&p2p.StatusMessage{HeadSlot: 100, FinalizedSlot: forkReconcileFinalizedThreshold + 1}) {
		t.Fatal("expected backfill when finalized is stuck behind a peer at equal head")
	}
	// Peer finalized only marginally ahead -> normal skew, no reconcile.
	if sd.shouldBackfill(&p2p.StatusMessage{HeadSlot: 100, FinalizedSlot: forkReconcileFinalizedThreshold - 1}) {
		t.Fatal("did not expect backfill for small finalized skew at equal head")
	}
}
