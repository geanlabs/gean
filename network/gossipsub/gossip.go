package gossipsub

import (
	"context"
	"fmt"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Gossip topic names.
const (
	BlockTopicFmt             = "/leanconsensus/%s/block/ssz_snappy"
	SubnetAttestationTopicFmt = "/leanconsensus/%s/attestation_%d/ssz_snappy"
	AggregationTopicFmt       = "/leanconsensus/%s/aggregation/ssz_snappy"
)

// Topics holds subscribed gossipsub topics.
type Topics struct {
	Block             *pubsub.Topic
	SubnetAttestation *pubsub.Topic
	Aggregation       *pubsub.Topic
}

// NewGossipSub creates a configured gossipsub instance.
// directPeers are always messaged regardless of mesh or subscription state (used for bootnodes).
func NewGossipSub(ctx context.Context, h host.Host, directPeers []peer.AddrInfo) (*pubsub.PubSub, error) {
	// Start from defaults to avoid zero-value bugs (WithGossipSubParams replaces
	// the entire struct, it does not merge), then override what we need.
	params := pubsub.DefaultGossipSubParams()
	params.D = 8
	params.Dlo = 6
	params.Dhi = 12
	params.Dlazy = 6
	params.HeartbeatInterval = 700 * time.Millisecond
	params.FanoutTTL = 60 * time.Second
	params.HistoryLength = 6
	params.HistoryGossip = 3
	params.GossipFactor = 0.25
	params.PruneBackoff = time.Minute
	params.UnsubscribeBackoff = 10 * time.Second
	params.Connectors = 8
	params.MaxPendingConnections = 128
	params.ConnectionTimeout = 30 * time.Second
	params.DirectConnectTicks = 300
	params.DirectConnectInitialDelay = time.Second
	params.OpportunisticGraftTicks = 60
	params.OpportunisticGraftPeers = 2
	params.GraftFloodThreshold = 10 * time.Second
	params.MaxIHaveLength = 5000
	params.MaxIHaveMessages = 10
	params.IWantFollowupTime = 3 * time.Second
	params.IDontWantMessageThreshold = 1000

	return pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(), // Omit author (From) and sequence number for anonymous mode compatibility
		pubsub.WithGossipSubParams(params),
		pubsub.WithSeenMessagesTTL(24*time.Second),
		pubsub.WithMessageIdFn(ComputeMessageID),
		pubsub.WithPeerOutboundQueueSize(256), // Larger outbound buffer prevents drops during attestation bursts
		pubsub.WithDirectPeers(directPeers),   // Always message bootnodes regardless of mesh/subscription state
	)
}

// JoinTopics joins the devnet-3 block, subnet attestation, and aggregation gossip topics.
func JoinTopics(ps *pubsub.PubSub, devnetID string, subnetID uint64) (*Topics, error) {
	blockTopic, err := ps.Join(fmt.Sprintf(BlockTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join block topic: %w", err)
	}
	subnetAttTopic, err := ps.Join(fmt.Sprintf(SubnetAttestationTopicFmt, devnetID, subnetID))
	if err != nil {
		return nil, fmt.Errorf("join subnet attestation topic: %w", err)
	}
	aggTopic, err := ps.Join(fmt.Sprintf(AggregationTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join aggregation topic: %w", err)
	}
	return &Topics{Block: blockTopic, SubnetAttestation: subnetAttTopic, Aggregation: aggTopic}, nil
}
