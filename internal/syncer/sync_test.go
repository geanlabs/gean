package syncer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestSyncStatus_String(t *testing.T) {
	cases := []struct {
		s    SyncStatus
		want string
	}{
		{SyncIdle, "idle"},
		{SyncSyncing, "syncing"},
		{SyncSynced, "synced"},
		{SyncStatus(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("SyncStatus(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

type mockSyncP2P struct {
	mu sync.Mutex

	peers []libp2ppeer.ID

	statusResp *p2p.StatusMessage
	statusErr  error

	rangeBatches [][]*types.SignedBlock // popped one per call; empty/nil returns (nil, rangeErr)
	rangeErr     error
	rangeCalls   atomic.Int32

	rootBlocks []*types.SignedBlock
	rootErr    error
	rootCalls  atomic.Int32

	// rangeBlock allows simulating a slow range fetch — if non-nil, the
	// FetchBlocksByRange call blocks on it until released by the test.
	rangeBlock chan struct{}
}

func (m *mockSyncP2P) Peers() []libp2ppeer.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peers
}

func (m *mockSyncP2P) SendStatusRequest(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) (*p2p.StatusMessage, error) {
	return m.statusResp, m.statusErr
}

func (m *mockSyncP2P) FetchBlocksByRange(ctx context.Context, peerID libp2ppeer.ID, startSlot, count uint64) ([]*types.SignedBlock, error) {
	m.rangeCalls.Add(1)
	if m.rangeBlock != nil {
		<-m.rangeBlock
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rangeBatches) == 0 {
		return nil, m.rangeErr
	}
	batch := m.rangeBatches[0]
	m.rangeBatches = m.rangeBatches[1:]
	return batch, m.rangeErr
}

func (m *mockSyncP2P) FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	m.rootCalls.Add(1)
	return m.rootBlocks, nil, m.rootErr
}

type testNode struct {
	status  SyncStatus
	BlockCh chan *types.SignedBlock
}

func (n *testNode) GetSyncStatus() SyncStatus { return n.status }

func (n *testNode) OnBlock(block *types.SignedBlock) {
	n.BlockCh <- block
}

func makeTestSyncHarness() (*testNode, *store.ConsensusStore) {
	store := store.NewConsensusStore(storage.NewInMemoryBackend())
	var root [32]byte
	root[0] = 1
	store.SetHead(root)
	store.InsertBlockHeader(root, &types.BlockHeader{Slot: 0})
	return &testNode{status: SyncSyncing, BlockCh: make(chan *types.SignedBlock, 16)}, store
}

func drainBlockCh(t *testing.T, n *testNode, expected int, timeout time.Duration) []*types.SignedBlock {
	t.Helper()
	var out []*types.SignedBlock
	deadline := time.After(timeout)
	for len(out) < expected {
		select {
		case b := <-n.BlockCh:
			out = append(out, b)
		case <-deadline:
			return out
		}
	}
	return out
}
func TestSyncDriver_CheckAndBackfill_PeerNotAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	// Engine head is at slot 0. Peer is also at slot 0 — not ahead.
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

	// Gap == BlocksByRangeSyncThreshold (boundary case, `<=` returns true)
	// — should NOT trigger range fetch.
	peerStatus := &p2p.StatusMessage{HeadSlot: p2p.BlocksByRangeSyncThreshold}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls for gap below threshold, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_FetchesWhenAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	// Peer returns blocks for the first request, then nothing — the loop
	// inside checkAndBackfill should drain once and stop on the empty reply.
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
				{Block: &types.Block{Slot: 2, Body: &types.BlockBody{}}},
			},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	// Gap of 100 > BlocksByRangeSyncThreshold (64). Should trigger range fetch.
	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	// One filled batch + one probe that gets the empty reply.
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
	// Peer drip-feeds two batches that together close the gap, then returns
	// empty. Drives the in-reservation loop instead of waiting one
	// SyncPollInterval (32s) between batches.
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
				{Block: &types.Block{Slot: 2, Body: &types.BlockBody{}}},
			},
			{
				{Block: &types.Block{Slot: 3, Body: &types.BlockBody{}}},
				{Block: &types.Block{Slot: 4, Body: &types.BlockBody{}}},
			},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	// Two filled batches + one probe that returns empty.
	if got := mock.rangeCalls.Load(); got != 3 {
		t.Errorf("expected 3 range calls (2 batches + empty probe), got %d", got)
	}

	got := drainBlockCh(t, n, 4, 100*time.Millisecond)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_FallsBackToRoot(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		rootBlocks: []*types.SignedBlock{
			{Block: &types.Block{Slot: 100, Body: &types.BlockBody{}}},
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
	if got := mock.rootCalls.Load(); got != 1 {
		t.Errorf("expected 1 root fallback call, got %d", got)
	}

	got := drainBlockCh(t, n, 1, 100*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 block from root fallback, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_PerPeerDedup(t *testing.T) {
	n, store := makeTestSyncHarness()
	rangeBlock := make(chan struct{})
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}}},
		},
		rangeBlock: rangeBlock,
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	peerID := libp2ppeer.ID("p1")

	// First call: should reserve the peer slot. Block is held by rangeBlock.
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	}()

	// Wait until the first call is in flight (range fetch entered).
	for mock.rangeCalls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}

	// Second concurrent call to the SAME peer: must NOT trigger a second
	// range fetch — the dedup map should reject it.
	sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call after dedup, got %d", got)
	}

	// Release the first call, drain its block.
	close(rangeBlock)
	<-done1
	_ = drainBlockCh(t, n, 1, 100*time.Millisecond)
}
