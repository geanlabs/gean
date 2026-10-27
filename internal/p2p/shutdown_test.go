package p2p

import (
	"context"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/types"
)

// Close must not return while a req/resp handler is still running: the
// handler reads the store, which the caller closes next.
func TestCloseWaitsForHeldReqRespHandler(t *testing.T) {
	server, client := newTestHost(t), newTestHost(t)

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

// heldValidator blocks in ValidateGossipBlock until released.
type heldValidator struct {
	captureHandler
	entered, release chan struct{}
}

func (h *heldValidator) ValidateGossipBlock(*types.SignedBlock) Verdict {
	close(h.entered)
	<-h.release
	return Accept
}

// Close must not return while pubsub is running a gossip validator, which
// reads the store, and ignores messages once it has begun.
func TestCloseWaitsForHeldGossipValidator(t *testing.T) {
	h := newTestHost(t)
	handler := &heldValidator{entered: make(chan struct{}), release: make(chan struct{})}
	h.StartGossipListeners(handler)

	topic := BlockTopic()
	msg := &pubsub.Message{Message: &pb.Message{Topic: &topic, Data: encodedGossip(t, gossipBlock(1))}}
	go h.validateGossip(context.Background(), peer.ID("remote"), msg)
	<-handler.entered

	closed := make(chan struct{})
	go func() {
		h.Close()
		close(closed)
	}()
	// Only detects an early return; passing never depends on the window.
	select {
	case <-closed:
		t.Fatal("Close returned while a gossip validator was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(handler.release)
	<-closed

	if got := h.validateGossip(context.Background(), peer.ID("remote"), msg); got != pubsub.ValidationIgnore {
		t.Fatalf("verdict after Close=%v, want ignore", got)
	}
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
