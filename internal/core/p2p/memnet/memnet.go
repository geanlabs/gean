// Package memnet is an in-memory libp2p network, for tests and simulations:
// its hosts reach each other without sockets. Every host is linked to every
// other, and peers are dialed by id; the addresses hosts are given are only
// labels.
package memnet

import (
	"fmt"
	"sync"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libp2phost "github.com/libp2p/go-libp2p/core/host"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/multiformats/go-multiaddr"
)

// Network is an in-memory network. It satisfies core/p2p/host.Network.
type Network struct {
	mu    sync.Mutex
	net   mocknet.Mocknet
	hosts int
}

// New returns an empty network.
func New() *Network {
	return &Network{net: mocknet.New()}
}

// NewHost adds a host with identity key and links it to every host already on
// the network.
func (n *Network) NewHost(key libp2pcrypto.PrivKey) (libp2phost.Host, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.hosts++
	label, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/10.0.%d.%d/udp/9000/quic-v1", n.hosts/256, n.hosts%256))
	if err != nil {
		return nil, err
	}
	h, err := n.net.AddPeer(key, label)
	if err != nil {
		return nil, fmt.Errorf("add in-memory host: %w", err)
	}
	for _, other := range n.net.Peers() {
		if other == h.ID() {
			continue
		}
		if _, err := n.net.LinkPeers(h.ID(), other); err != nil {
			return nil, fmt.Errorf("link in-memory hosts: %w", err)
		}
	}
	return h, nil
}

// Close closes every host on the network.
func (n *Network) Close() error {
	return n.net.Close()
}
