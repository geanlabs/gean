package memnet

import (
	"context"
	"crypto/rand"
	"io"
	"testing"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func newKey(t *testing.T) libp2pcrypto.PrivKey {
	t.Helper()
	key, _, err := libp2pcrypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// Hosts added at different times reach each other by id, and a stream between
// them carries data both ways.
func TestHostsConnectAndStream(t *testing.T) {
	n := New()
	defer n.Close()
	a, err := n.NewHost(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := n.NewHost(newKey(t))
	if err != nil {
		t.Fatal(err)
	}

	const proto = "/test/echo/1"
	b.SetStreamHandler(proto, func(s network.Stream) {
		defer s.Close()
		io.Copy(s, s)
	})

	ctx := context.Background()
	if err := a.Connect(ctx, peer.AddrInfo{ID: b.ID(), Addrs: b.Addrs()}); err != nil {
		t.Fatal(err)
	}
	s, err := a.NewStream(ctx, b.ID(), proto)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	s.CloseWrite()
	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ping" {
		t.Fatalf("echo=%q, want ping", got)
	}
}
