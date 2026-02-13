package gossipsub

import (
	"encoding/hex"
	"testing"

	"github.com/golang/snappy"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

// Test vectors from zeam (Zig client) at zeam/rust/src/libp2p_bridge.rs.

func TestComputeMessageIDValidSnappy(t *testing.T) {
	// zeam test: snappy-compress "hello", topic "test"
	// Expected: "2e40c861545cc5b46d2220062e7440b9190bc383"
	compressed := snappy.Encode(nil, []byte("hello"))
	topic := "test"

	msg := &pb.Message{
		Data:  compressed,
		Topic: &topic,
	}

	id := ComputeMessageID(msg)
	got := hex.EncodeToString([]byte(id))
	expected := "2e40c861545cc5b46d2220062e7440b9190bc383"
	if got != expected {
		t.Errorf("valid snappy message ID mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestComputeMessageIDInvalidSnappy(t *testing.T) {
	// zeam test: raw "hello" (not snappy compressed), topic "test"
	// Expected: "a7f41aaccd241477955c981714eb92244c2efc98"
	topic := "test"

	msg := &pb.Message{
		Data:  []byte("hello"),
		Topic: &topic,
	}

	id := ComputeMessageID(msg)
	got := hex.EncodeToString([]byte(id))
	expected := "a7f41aaccd241477955c981714eb92244c2efc98"
	if got != expected {
		t.Errorf("invalid snappy message ID mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}
