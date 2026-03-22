package gossipsub

import (
	"context"
	"time"

	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

var gossipLog = logging.NewComponentLogger(logging.CompGossip)

// GossipHandler processes decoded gossip messages.
type GossipHandler struct {
	OnBlock                 func(*types.SignedBlockWithAttestation)
	OnAttestation           func(*types.SignedAttestation)
	OnAggregatedAttestation func(*types.SignedAggregatedAttestation)
}

// SubscribeTopics subscribes to topics and dispatches messages to handler.
func SubscribeTopics(ctx context.Context, topics *Topics, handler *GossipHandler) error {
	blockSub, err := topics.Block.Subscribe()
	if err != nil {
		gossipLog.Error("failed to subscribe to block topic", "err", err)
		return err
	}
	attSub, err := topics.SubnetAttestation.Subscribe()
	if err != nil {
		gossipLog.Error("failed to subscribe to attestation topic", "err", err)
		return err
	}
	aggSub, err := topics.Aggregation.Subscribe()
	if err != nil {
		gossipLog.Error("failed to subscribe to aggregation topic", "err", err)
		return err
	}

	gossipLog.Info("subscribed to gossip topics",
		"block_topic", topics.Block.String(),
	)
	go readBlockMessages(ctx, blockSub, topics.Block, handler)
	go readAttestationMessages(ctx, attSub, topics.SubnetAttestation, handler)
	go readAggregatedAttestationMessages(ctx, aggSub, handler)
	return nil
}

func readBlockMessages(ctx context.Context, sub *pubsub.Subscription, topic *pubsub.Topic, handler *GossipHandler) {
	// Log mesh peers periodically to diagnose gossip issues.
	meshLogTicker := time.NewTicker(12 * time.Second)
	defer meshLogTicker.Stop()

	for {
		select {
		case <-meshLogTicker.C:
			peers := topic.ListPeers()
			gossipLog.Info("block topic mesh state",
				"mesh_peers", len(peers),
				"topic", topic.String(),
			)
		default:
		}

		msg, err := sub.Next(ctx)
		if err != nil {
			gossipLog.Error("block subscription ended", "err", err)
			return
		}

		// Log source peer to help debug mesh issues.
		fromPeer := msg.ReceivedFrom.String()

		decoded, err := snappy.Decode(nil, msg.Data)
		if err != nil {
			gossipLog.Warn("failed to snappy decode block", "from", fromPeer, "err", err)
			continue
		}
		block := new(types.SignedBlockWithAttestation)
		if err := block.UnmarshalSSZ(decoded); err != nil {
			gossipLog.Warn("failed to unmarshal block", "from", fromPeer, "err", err)
			continue
		}

		gossipLog.Debug("block message received", "from", fromPeer, "slot", block.Message.Block.Slot)

		if handler.OnBlock != nil {
			handler.OnBlock(block)
		}
	}
}

func readAttestationMessages(ctx context.Context, sub *pubsub.Subscription, topic *pubsub.Topic, handler *GossipHandler) {
	meshLogTicker := time.NewTicker(12 * time.Second)
	defer meshLogTicker.Stop()

	for {
		select {
		case <-meshLogTicker.C:
			peers := topic.ListPeers()
			gossipLog.Info("attestation topic mesh state",
				"mesh_peers", len(peers),
				"topic", topic.String(),
			)
		default:
		}

		msg, err := sub.Next(ctx)
		if err != nil {
			gossipLog.Error("attestation subscription ended", "err", err)
			return
		}
		decoded, err := snappy.Decode(nil, msg.Data)
		if err != nil {
			continue
		}
		att := new(types.SignedAttestation)
		if err := att.UnmarshalSSZ(decoded); err != nil {
			continue
		}
		if handler.OnAttestation != nil {
			handler.OnAttestation(att)
		}
	}
}

func readAggregatedAttestationMessages(ctx context.Context, sub *pubsub.Subscription, handler *GossipHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			gossipLog.Error("aggregation subscription ended", "err", err)
			return
		}
		decoded, err := snappy.Decode(nil, msg.Data)
		if err != nil {
			continue
		}
		agg := new(types.SignedAggregatedAttestation)
		if err := agg.UnmarshalSSZ(decoded); err != nil {
			continue
		}
		if handler.OnAggregatedAttestation != nil {
			handler.OnAggregatedAttestation(agg)
		}
	}
}
