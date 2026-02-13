package gossipsub

import (
	"context"
	"fmt"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

// Gossip topic names.
const (
	BlockTopicFmt = "/leanconsensus/%s/block/ssz_snappy"
	AttestationTopicFmt = "/leanconsensus/%s/attestation/ssz_snappy"
)

// Topics holds subscribed gossipsub topics.
type Topics struct {
	Block       *pubsub.Topic
	Attestation *pubsub.Topic
}

// NewGossipSub creates a configured gossipsub instance.
func NewGossipSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	return pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
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
	)
}

// JoinTopics joins the block and vote gossip topics.
func JoinTopics(ps *pubsub.PubSub, devnetID string) (*Topics, error) {
	blockTopic, err := ps.Join(fmt.Sprintf(BlockTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join block topic: %w", err)
	}
	attTopic, err := ps.Join(fmt.Sprintf(AttestationTopicFmt, devnetID))
	if err != nil {
		return nil, fmt.Errorf("join attestation topic: %w", err)
	}
	return &Topics{Block: blockTopic, Attestation: attTopic}, nil
}
