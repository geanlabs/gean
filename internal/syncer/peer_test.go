package syncer

import (
	"context"
	"testing"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
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

func TestSyncDriverNilDependenciesReturnDefaults(t *testing.T) {
	sd := NewSyncDriver(context.Background(), nil, nil, nil)
	peerID := libp2ppeer.ID("p1")

	if sd.makeStatusMessage() != nil {
		t.Fatal("nil store should not build a status message")
	}
	if sd.shouldBackfill(&p2p.StatusMessage{HeadSlot: 10}) {
		t.Fatal("nil store should not backfill")
	}

	sd.Run()
	sd.OnPeerConnected(peerID)
	sd.refreshSyncFromPeers(context.Background())
	sd.pollPeer(context.Background(), peerID, nil)
	sd.checkAndBackfill(context.Background(), peerID, &p2p.StatusMessage{HeadSlot: 10})

	var nilDriver *SyncDriver
	nilDriver.Run()
	nilDriver.OnPeerConnected(peerID)
	nilDriver.refreshSyncFromPeers(context.Background())
	nilDriver.pollPeer(context.Background(), peerID, nil)
	nilDriver.checkAndBackfill(context.Background(), peerID, &p2p.StatusMessage{HeadSlot: 10})
	if nilDriver.makeStatusMessage() != nil {
		t.Fatal("nil driver should not build a status message")
	}
	if nilDriver.shouldBackfill(&p2p.StatusMessage{HeadSlot: 10}) {
		t.Fatal("nil driver should not backfill")
	}
}
