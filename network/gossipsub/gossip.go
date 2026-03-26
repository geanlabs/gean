package gossipsub

import (
	"context"
	"fmt"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const NetworkName = "devnet0"

func AttestationSubnetTopic(subnetID uint64) string {
	return fmt.Sprintf(SubnetAttestationTopicFmt, NetworkName, subnetID)
}

// Gossip topic names.
const (
	BlockTopicFmt             = "/leanconsensus/%s/block/ssz_snappy"
	SubnetAttestationTopicFmt = "/leanconsensus/%s/attestation_%d/ssz_snappy"
	AggregationTopicFmt       = "/leanconsensus/%s/aggregation/ssz_snappy"
)

// Topics holds subscribed gossipsub topics.
type Topics struct {
	Block                     *pubsub.Topic
	SubnetAttestations        map[uint64]*pubsub.Topic
	Aggregation               *pubsub.Topic
	AttestationCommitteeCount int
	PubSub                    *pubsub.PubSub
}

// NewGossipSub creates a configured gossipsub instance.
// directPeers are always messaged regardless of mesh or subscription state (used for bootnodes).
func NewGossipSub(ctx context.Context, h host.Host, directPeers []peer.AddrInfo) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(), // Omit author (From) and sequence number for anonymous mode compatibility
		pubsub.WithGossipSubParams(pubsub.GossipSubParams{
			D:                         8,
			Dlo:                       6,
			Dhi:                       12,
			Dlazy:                     6,
			HeartbeatInterval:         700 * time.Millisecond,
			FanoutTTL:                 60 * time.Second,
			HistoryLength:             6,
			HistoryGossip:             3,
			GossipFactor:              0.25,
			PruneBackoff:              time.Minute,
			UnsubscribeBackoff:        10 * time.Second,
			Connectors:                8,
			MaxPendingConnections:     128,
			ConnectionTimeout:         30 * time.Second,
			DirectConnectTicks:        300,
			DirectConnectInitialDelay: time.Second,
			OpportunisticGraftTicks:   60,
			OpportunisticGraftPeers:   2,
			GraftFloodThreshold:       10 * time.Second,
			MaxIHaveLength:            5000,
			MaxIHaveMessages:          10,
			IWantFollowupTime:         3 * time.Second,
		}),
		pubsub.WithSeenMessagesTTL(24*time.Second),
		pubsub.WithMessageIdFn(ComputeMessageID),
		pubsub.WithFloodPublish(true),       // Send to all subscribed peers, bypassing mesh for small devnets
		pubsub.WithDirectPeers(directPeers), // Always message bootnodes regardless of mesh/subscription state
	)
}

// JoinTopics joins the block, specified subnet attestation, and aggregation gossip topics.
func JoinTopics(ps *pubsub.PubSub, devnetID string, subnetIDs map[uint64]bool, committeeCount int) (*Topics, error) {
	blockTopic, err := ps.Join(fmt.Sprintf(BlockTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join block topic: %w", err)
	}

	subnetAttestations := make(map[uint64]*pubsub.Topic)
	for subnetID := range subnetIDs {
		topic, err := ps.Join(fmt.Sprintf(SubnetAttestationTopicFmt, devnetID, subnetID))
		if err != nil {
			return nil, fmt.Errorf("join subnet attestation topic %d: %w", subnetID, err)
		}
		subnetAttestations[subnetID] = topic
	}

	aggTopic, err := ps.Join(fmt.Sprintf(AggregationTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join aggregation topic: %w", err)
	}
	return &Topics{
		Block:                     blockTopic,
		SubnetAttestations:        subnetAttestations,
		Aggregation:               aggTopic,
		AttestationCommitteeCount: committeeCount,
		PubSub:                    ps,
	}, nil
}
