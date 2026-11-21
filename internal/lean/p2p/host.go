package p2p

import (
	"context"
	"sync/atomic"

	"github.com/geanlabs/gean/internal/core/p2p/host"
)

// Host is the generic libp2p host carrying lean's topics, gossip validation
// and req/resp protocols.
type Host struct {
	*host.Host

	// gossipHandler is read by pubsub's validation goroutines, so it is set once
	// atomically when the listeners start.
	gossipHandler  atomic.Pointer[MessageHandler]
	committeeCount uint64
	GossipHooks    GossipHooks
}

func NewHost(
	ctx context.Context,
	hostCfg host.Config,
	committeeCount uint64,
	validatorIDs []uint64,
	isAggregator bool,
	aggregateSubnetIDs []uint64,
) (*Host, error) {
	hostCfg.DiscoveryAdmit = SameFork
	base, err := host.New(ctx, hostCfg)
	if err != nil {
		return nil, err
	}
	p2pHost := &Host{Host: base, committeeCount: committeeCount}
	p2pHost.SetENREntry(localEth2)

	if err := p2pHost.subscribeStartupTopics(committeeCount, validatorIDs, isAggregator, aggregateSubnetIDs); err != nil {
		p2pHost.Close()
		return nil, err
	}

	return p2pHost, nil
}
