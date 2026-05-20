package node

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/types"
)

// --- SyncStatus enum ---

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

// --- mockSyncP2P implements SyncDriverP2P for testing checkAndBackfill ---

type mockSyncP2P struct {
	mu sync.Mutex

	peers []libp2ppeer.ID

	statusResp *p2p.StatusMessage
	statusErr  error

	rangeBlocks []*types.SignedBlock
	rangeErr    error
	rangeCalls  atomic.Int32

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
	return m.rangeBlocks, m.rangeErr
}

func (m *mockSyncP2P) FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	m.rootCalls.Add(1)
	return m.rootBlocks, nil, m.rootErr
}

// --- helper: drain BlockCh into a slice for assertions ---

func drainBlockCh(t *testing.T, e *Engine, expected int, timeout time.Duration) []*types.SignedBlock {
	t.Helper()
	var out []*types.SignedBlock
	deadline := time.After(timeout)
	for len(out) < expected {
		select {
		case b := <-e.BlockCh:
			out = append(out, b)
		case <-deadline:
			return out
		}
	}
	return out
}

// --- checkAndBackfill tests ---

func TestSyncDriver_CheckAndBackfill_PeerNotAhead(t *testing.T) {
	e := makeTestEngine()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), e, mock)

	// Engine head is at slot 0. Peer is also at slot 0 — not ahead.
	peerStatus := &p2p.StatusMessage{HeadSlot: 0}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls when peer not ahead, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_GapBelowThreshold(t *testing.T) {
	e := makeTestEngine()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), e, mock)

	// Gap == BlocksByRangeSyncThreshold (boundary case, `<=` returns true)
	// — should NOT trigger range fetch.
	peerStatus := &p2p.StatusMessage{HeadSlot: p2p.BlocksByRangeSyncThreshold}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls for gap below threshold, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_FetchesWhenAhead(t *testing.T) {
	e := makeTestEngine()
	mock := &mockSyncP2P{
		rangeBlocks: []*types.SignedBlock{
			{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
			{Block: &types.Block{Slot: 2, Body: &types.BlockBody{}}},
		},
	}
	sd := NewSyncDriver(context.Background(), e, mock)

	// Gap of 100 > BlocksByRangeSyncThreshold (64). Should trigger range fetch.
	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call, got %d", got)
	}

	got := drainBlockCh(t, e, 2, 100*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_FallsBackToRoot(t *testing.T) {
	e := makeTestEngine()
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		rootBlocks: []*types.SignedBlock{
			{Block: &types.Block{Slot: 100, Body: &types.BlockBody{}}},
		},
	}
	sd := NewSyncDriver(context.Background(), e, mock)

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

	got := drainBlockCh(t, e, 1, 100*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 block from root fallback, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_PerPeerDedup(t *testing.T) {
	e := makeTestEngine()
	rangeBlock := make(chan struct{})
	mock := &mockSyncP2P{
		rangeBlocks: []*types.SignedBlock{
			{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
		},
		rangeBlock: rangeBlock,
	}
	sd := NewSyncDriver(context.Background(), e, mock)

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
	_ = drainBlockCh(t, e, 1, 100*time.Millisecond)
}

// --- Run loop sync-status gating ---

func TestSyncDriver_GetSyncStatus_NoPeers(t *testing.T) {
	// makeTestEngine constructs Engine with P2P=nil, so the no-peers check
	// is skipped. To exercise SyncIdle, we'd need a real Host with zero
	// peers. Instead verify the computeSyncStatus branch directly.
	e := makeTestEngine()

	// With P2P=nil, computeSyncStatus skips the no-peers check and returns
	// based on head-vs-currentSlot. Head is at slot 0, currentSlot is some
	// large value (genesis was set to ms 1000, so we're far ahead now).
	// Expect SyncSyncing.
	got := e.computeSyncStatus(uint64(10000))
	if got != SyncSyncing {
		t.Errorf("expected SyncSyncing for head=0 currentSlot=10000, got %v", got)
	}

	// With currentSlot near head, expect SyncSynced.
	got = e.computeSyncStatus(uint64(0))
	if got != SyncSynced {
		t.Errorf("expected SyncSynced for head=0 currentSlot=0, got %v", got)
	}
}
