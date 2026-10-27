package p2p

import (
	"context"
	"fmt"
	"sync/atomic"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/geanlabs/gean/internal/lifecycle"
)

type Host struct {
	host      host.Host
	pubsub    *pubsub.PubSub
	topics    map[string]*pubsub.Topic
	subs      map[string]*pubsub.Subscription
	ctx       context.Context
	cancel    context.CancelFunc
	peerStore *PeerStore
	// gossipHandler is read by pubsub's validation goroutines, so it is set once
	// atomically when the listeners start.
	gossipHandler  atomic.Pointer[MessageHandler]
	committeeCount uint64
	Hooks          Hooks

	// tasks tracks the goroutines the host starts, the req/resp handlers it
	// runs and the gossip validators pubsub calls, which read the store, so
	// Close can join them.
	tasks lifecycle.Group
}

func NewHost(
	ctx context.Context,
	nodeKeyPath string,
	listenPort int,
	committeeCount uint64,
	validatorIDs []uint64,
	isAggregator bool,
	aggregateSubnetIDs []uint64,
) (*Host, error) {
	ctx, cancel := context.WithCancel(ctx)

	privKey, err := loadNodeKey(nodeKeyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load node key: %w", err)
	}

	libp2pHost, err := newLibP2PHost(privKey, listenPort)
	if err != nil {
		cancel()
		return nil, err
	}

	ps, err := newGossipSub(ctx, libp2pHost)
	if err != nil {
		libp2pHost.Close()
		cancel()
		return nil, err
	}

	p2pHost := &Host{
		host:           libp2pHost,
		pubsub:         ps,
		topics:         make(map[string]*pubsub.Topic),
		subs:           make(map[string]*pubsub.Subscription),
		ctx:            ctx,
		cancel:         cancel,
		peerStore:      NewPeerStore(),
		committeeCount: committeeCount,
	}
	p2pHost.installPeerNotifier()

	if err := p2pHost.subscribeStartupTopics(committeeCount, validatorIDs, isAggregator, aggregateSubnetIDs); err != nil {
		p2pHost.Close()
		return nil, err
	}

	return p2pHost, nil
}
