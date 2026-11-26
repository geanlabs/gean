package host

import (
	"context"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
)

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

// A zero seen TTL would let duplicate gossip through, so it is refused.
func TestNewRefusesZeroSeenTTL(t *testing.T) {
	_, err := New(context.Background(), Config{})
	if err == nil || !strings.Contains(err.Error(), "seen TTL") {
		t.Fatalf("New with no SeenTTL: err = %v, want a seen TTL error", err)
	}
}
