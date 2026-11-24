// Package host is a libp2p node for consensus networks: its identity, QUIC
// transport and gossipsub instance, the topics it joins, the peers it tracks,
// and the goroutines run on it, which Close joins. The chain's topics,
// protocols and message handling are its users'.
package host

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
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
	// discovery is discv5, when StartDiscovery has run. enrMu guards it and
	// enrEntries, the entries set before it started.
	enrMu      sync.Mutex
	discovery  *discovery.Service
	enrEntries map[string]enr.Entry
	// hooks are fixed at construction: libp2p calls the peer notifier on its own
	// goroutines as soon as the host can accept connections.
	hooks Hooks
	// peerStatus is set once the node can sync, after the host is accepting
	// connections, so it is stored atomically.
	peerStatus atomic.Pointer[func(peer.ID)]

	discoveryAdmit func(*enode.Node) bool

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
	// SeenTTL is how long gossip remembers a message id, and must be positive.
	// The chain derives it from its slot timing: two epochs on the beacon chain.
	SeenTTL time.Duration
	// DiscoveryAdmit, when set, decides which nodes discv5 finds are dialed:
	// those on the chain's network.
	DiscoveryAdmit func(*enode.Node) bool
	// Hooks are fixed for the host's life.
	Hooks Hooks
}

// New starts a host with the identity in cfg.NodeKeyPath, listening for QUIC
// on cfg.ListenPort or joining cfg.Network.
func New(ctx context.Context, cfg Config) (*Host, error) {
	// pubsub would accept zero and drop each id at its next minutely sweep,
	// letting duplicates through.
	if cfg.SeenTTL <= 0 {
		return nil, fmt.Errorf("gossip seen TTL must be positive, got %v", cfg.SeenTTL)
	}
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

	ps, err := gossip.NewPubSub(ctx, libp2pHost, cfg.SeenTTL)
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

		discoveryAdmit: cfg.DiscoveryAdmit,
		hooks:          cfg.Hooks,
	}
	h.installPeerNotifier()
	return h, nil
}
