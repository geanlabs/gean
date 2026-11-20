package p2p

import (
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

// Two lean hosts given no bootnode address find each other through discv5:
// the second starts from the first's ENR and dials the QUIC address in it.
func TestHostsFindEachOtherThroughDiscv5(t *testing.T) {
	a, b := newTestHost(t, nil), newTestHost(t, nil)
	loopback := net.IPv4(127, 0, 0, 1)
	if err := a.StartDiscovery(0, loopback, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.StartDiscovery(0, loopback, []*enode.Node{a.Node()}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the hosts to connect", func() bool {
		return a.ConnectedPeers() > 0 && b.ConnectedPeers() > 0
	})
}
