package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/p2p/host"
	"github.com/geanlabs/gean/internal/core/p2p/memnet"
	"github.com/geanlabs/gean/internal/lean/types"
)

func newMemnetHost(t *testing.T, n *memnet.Network) *Host {
	t.Helper()
	priv, _, err := libp2pcrypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.Raw()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "node.key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := NewHost(context.Background(), host.Config{NodeKeyPath: path, Network: n}, 1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

// Lean's gossip and req/resp work unchanged over the in-memory network.
func TestLeanProtocolsOverMemnet(t *testing.T) {
	n := memnet.New()
	defer n.Close()
	a, b := newMemnetHost(t, n), newMemnetHost(t, n)

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
