// Package host is a libp2p node for consensus networks: its identity, QUIC
// transport and gossipsub instance, the topics it joins, the peers it tracks,
// and the goroutines run on it, which Close joins. The chain's topics,
// protocols and message handling are its users'.
package host

import (
	"context"
	"fmt"
	"sync/atomic"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/lifecycle"
	"github.com/geanlabs/gean/internal/core/p2p/discovery"
	"github.com/geanlabs/gean/internal/core/p2p/gossip"
	"github.com/geanlabs/gean/internal/core/p2p/peers"
)

// Hooks are the node's reactions to peer events. Each may be nil.
type Hooks struct {
	PeerConnected    func(direction string)
	PeerDisconnected func(direction, reason string)
	PeerCount        func(count int)
	// PeerStatus runs as one of the host's tasks for each new outbound peer.
	PeerStatus func(peerID peer.ID)
}

type Host struct {
	host      libp2phost.Host
	pubsub    *pubsub.PubSub
	topics    map[string]*pubsub.Topic
	subs      map[string]*pubsub.Subscription
	ctx       context.Context
	cancel    context.CancelFunc
	peerStore *peers.Store
	// deliver is set once, when the listeners start, and read again when
	// subscriptions are re-announced.
	deliver atomic.Pointer[Deliver]
	// discovery is discv5, when StartDiscovery has run.
	discovery *discovery.Service
	Hooks     Hooks

	// tasks tracks the goroutines the host starts, the req/resp handlers it
	// runs and the gossip validators pubsub calls, which read the store, so
	// Close can join them.
	tasks lifecycle.Group
}

// Network builds libp2p hosts on a network other than the real one, such as
// an in-memory network for tests and simulations.
type Network interface {
	NewHost(key libp2pcrypto.PrivKey) (libp2phost.Host, error)
}

// Config is what a host needs.
type Config struct {
	// NodeKeyPath is the hex secp256k1 key file holding the node's identity.
	NodeKeyPath string
	// ListenPort is the UDP port QUIC listens on.
	ListenPort int
	// Network, when set, builds the host on it instead of on QUIC.
	Network Network
}

// New starts a host with the identity in cfg.NodeKeyPath, listening for QUIC
// on cfg.ListenPort or joining cfg.Network.
func New(ctx context.Context, cfg Config) (*Host, error) {
	ctx, cancel := context.WithCancel(ctx)

	privKey, err := loadNodeKey(cfg.NodeKeyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load node key: %w", err)
	}

	var libp2pHost libp2phost.Host
	if cfg.Network != nil {
		libp2pHost, err = cfg.Network.NewHost(privKey)
	} else {
		libp2pHost, err = newLibP2PHost(privKey, cfg.ListenPort)
	}
	if err != nil {
		cancel()
		return nil, err
	}

	ps, err := gossip.NewPubSub(ctx, libp2pHost)
	if err != nil {
		libp2pHost.Close()
		cancel()
		return nil, err
	}

	h := &Host{
		host:      libp2pHost,
		pubsub:    ps,
		topics:    make(map[string]*pubsub.Topic),
		subs:      make(map[string]*pubsub.Subscription),
		ctx:       ctx,
		cancel:    cancel,
		peerStore: peers.NewStore(),
	}
	h.installPeerNotifier()
	return h, nil
}
