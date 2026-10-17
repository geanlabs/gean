package p2p

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/types"
)

func newTestHost(t *testing.T) *Host {
	t.Helper()
	key, _, err := libp2pcrypto.GenerateSecp256k1Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := key.Raw()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "node.key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := NewHost(context.Background(), path, 0, 1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// Close must not return while a req/resp handler is still running: the
// handler reads the store, which the caller closes next.
func TestCloseWaitsForHeldReqRespHandler(t *testing.T) {
	server, client := newTestHost(t), newTestHost(t)
	defer client.Close()

	entered, release := make(chan struct{}), make(chan struct{})
	server.RegisterReqRespHandlers(
		func() *StatusMessage {
			close(entered)
			<-release
			return &StatusMessage{}
		},
		func([32]byte) *types.SignedBlock { return nil },
		func() uint64 { return 0 },
		func(uint64, uint64) ([]*types.SignedBlock, bool) { return nil, false },
	)

	ctx := context.Background()
	if err := client.host.Connect(ctx, peer.AddrInfo{ID: server.PeerID(), Addrs: server.Addrs()}); err != nil {
		t.Fatal(err)
	}
	go client.SendStatusRequest(ctx, server.PeerID(), &StatusMessage{})
	<-entered

	closed := make(chan struct{})
	go func() {
		server.Close()
		close(closed)
	}()
	// Only detects an early return; passing never depends on the window.
	select {
	case <-closed:
		t.Fatal("Close returned while a req/resp handler was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-closed
}

type resetStream struct {
	network.Stream
	reset bool
}

func (s *resetStream) Reset() error {
	s.reset = true
	return nil
}

// A stream arriving once Close has begun is reset without being handled.
func TestTrackedHandlerAfterCloseResetsStream(t *testing.T) {
	h := &Host{}
	h.tasks.Close()

	s := &resetStream{}
	h.tracked(func(network.Stream) { t.Error("handler ran after Close") })(s)
	if !s.reset {
		t.Fatal("stream not reset")
	}
}
