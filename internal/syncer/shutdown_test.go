package syncer

import (
	"context"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

func backfillingDriver(ctx context.Context) (*SyncDriver, *testNode, *mockSyncP2P) {
	n, st := makeTestSyncHarness()
	mock := &mockSyncP2P{
		peers:      []libp2ppeer.ID{"p1"},
		statusResp: &p2p.StatusMessage{HeadSlot: 100},
	}
	return NewSyncDriver(ctx, n, st, mock), n, mock
}

// Wait must not return while a poll is inside a call that ignores
// cancellation: the poll still reads the store when it resumes.
func TestWaitJoinsPollHeldInFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sd, _, mock := backfillingDriver(ctx)
	mock.rangeEntered = make(chan struct{})
	mock.rangeBlock = make(chan struct{})

	sd.refreshSyncFromPeers(ctx)
	<-mock.rangeEntered

	cancel()
	waited := make(chan struct{})
	go func() {
		sd.Wait()
		close(waited)
	}()
	// Only detects an early return; passing never depends on the window.
	select {
	case <-waited:
		t.Fatal("Wait returned while a poll was still fetching")
	case <-time.After(100 * time.Millisecond):
	}
	close(mock.rangeBlock)
	<-waited
}

// A poll stalled delivering into a full block queue must return on cancel.
func TestWaitReturnsWithBlockQueueFull(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sd, n, mock := backfillingDriver(ctx)
	n.BlockCh = make(chan *types.SignedBlock, 1)
	n.BlockCh <- nil // nothing drains it
	mock.rangeEntered = make(chan struct{})
	mock.rangeBatches = [][]*types.SignedBlock{makeSyncRange(sd.store.Head(), 1, 2)}

	sd.refreshSyncFromPeers(ctx)
	<-mock.rangeEntered // the delivery that follows blocks on the full queue

	cancel()
	sd.Wait()
}

// After Wait, a new peer must not start a status check against the store.
func TestOnPeerConnectedAfterWaitDoesNothing(t *testing.T) {
	sd, _, mock := backfillingDriver(context.Background())
	sd.Wait()

	sd.OnPeerConnected("p1")
	if got := mock.rangeCalls.Load(); got != 0 {
		t.Fatalf("range calls=%d after Wait, want 0", got)
	}
}
