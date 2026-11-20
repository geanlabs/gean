package syncer

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/lean/p2p"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestSyncDriver_PollPeerIgnoresNilStatus(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.pollPeer(context.Background(), libp2ppeer.ID("p1"), sd.makeStatusMessage())

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Fatalf("nil peer status should not start backfill, got %d range calls", got)
	}
}

// A backfill started by a new connection must not inherit a deadline: a
// catch-up longer than the deadline, or one retry after a stalled request,
// would otherwise be cut off and wait for the next poll.
func TestSyncDriver_OnPeerConnectedBackfillHasNoDeadline(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		statusResp:   &p2p.StatusMessage{HeadSlot: 4},
		rangeBatches: [][]*types.SignedBlock{makeSyncRange(store.Head(), 1, 2, 3, 4)},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.OnPeerConnected(libp2ppeer.ID("p1"))

	if mock.rangeCalls.Load() == 0 {
		t.Fatal("connect did not start a backfill")
	}
	if mock.rangeDeadline.Load() {
		t.Fatal("connect-triggered backfill ran under a deadline")
	}
}

// A marooned node's head keeps pace, so nothing local says it needs to sync. The
// periodic poll is what fetches the peer status that reveals it.
func TestRunPollsMaroonedNodeWithCurrentHead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		n, st := makeTestSyncHarness()
		mock := &mockSyncP2P{
			peers:      []libp2ppeer.ID{"p1"},
			statusResp: &p2p.StatusMessage{FinalizedSlot: forkReconcileFinalizedThreshold + 1},
		}
		st.SetLatestFinalized(&types.Checkpoint{Slot: 0})
		sd := NewSyncDriver(ctx, n, st, mock)
		sd.Start()

		time.Sleep(pollInterval + time.Second)
		synctest.Wait()
		cancel()
		sd.Wait()

		if mock.statusCalls.Load() == 0 {
			t.Fatal("periodic poll did not request peer status")
		}
	})
}
