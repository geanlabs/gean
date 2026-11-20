package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/p2p/memnet"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Lean's gossip and req/resp work unchanged over the in-memory network.
func TestLeanProtocolsOverMemnet(t *testing.T) {
	n := memnet.New()
	defer n.Close()
	a, b := newTestHost(t, n), newTestHost(t, n)

	b.RegisterReqRespHandlers(
		func() *StatusMessage { return &StatusMessage{HeadSlot: 7} },
		func([32]byte) *types.SignedBlock { return nil },
		func() uint64 { return 0 },
		func(uint64, uint64) ([]*types.SignedBlock, bool) { return nil, false },
	)
	received := &captureHandler{verdict: Accept}
	b.StartGossipListeners(received)
	a.StartGossipListeners(&captureHandler{verdict: Accept})

	ctx := context.Background()
	if err := a.LibP2PHost().Connect(ctx, peer.AddrInfo{ID: b.PeerID(), Addrs: b.Addrs()}); err != nil {
		t.Fatal(err)
	}

	status, err := a.SendStatusRequest(ctx, b.PeerID(), &StatusMessage{})
	if err != nil {
		t.Fatal(err)
	}
	if status.HeadSlot != 7 {
		t.Fatalf("status head slot=%d, want 7", status.HeadSlot)
	}

	// Gossipsub drops a publish made before the mesh forms; probe until one
	// block arrives.
	waitFor(t, "a gossip block to cross the in-memory network", func() bool {
		if err := a.Publish(ctx, BlockTopic(), encodedGossip(t, gossipBlock(uint64(time.Now().UnixNano())))); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		return len(received.receivedSlots()) > 0
	})
}
