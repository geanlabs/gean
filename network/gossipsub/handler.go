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

const (
	blockBufferSize       = 16
	attestationBufferSize = 256
	aggregationBufferSize = 64
)

// GossipHandler processes decoded gossip messages.
type GossipHandler struct {
	OnBlock                 func(*types.SignedBlockWithAttestation)
	OnAttestation           func(*types.SignedAttestation)
	OnAggregatedAttestation func(*types.SignedAggregatedAttestation)
}

// SubscribeTopics subscribes to topics and dispatches messages to handler.
// Reader goroutines decode messages and enqueue them into buffered channels.
// Separate consumer goroutines dequeue and call the handler callbacks, so
// slow processing (e.g. sync recovery) never blocks the gossipsub readers.
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

	blockCh := make(chan *types.SignedBlockWithAttestation, blockBufferSize)
	attCh := make(chan *types.SignedAttestation, attestationBufferSize)
	aggCh := make(chan *types.SignedAggregatedAttestation, aggregationBufferSize)

	// Readers: decode and enqueue (non-blocking).
	go readBlockMessages(ctx, blockSub, topics.Block, blockCh)
	go readAttestationMessages(ctx, attSub, topics.SubnetAttestation, attCh)
	go readAggregatedAttestationMessages(ctx, aggSub, aggCh)

	// Consumers: dequeue and process (may block on forkchoice lock or sync).
	go processBlocks(ctx, blockCh, handler)
	go processAttestations(ctx, attCh, handler)
	go processAggregatedAttestations(ctx, aggCh, handler)

	return nil
}

func readBlockMessages(ctx context.Context, sub *pubsub.Subscription, topic *pubsub.Topic, out chan<- *types.SignedBlockWithAttestation) {
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

		select {
		case out <- block:
		default:
			gossipLog.Warn("block processing channel full, dropping message",
				"slot", block.Message.Block.Slot,
				"from", fromPeer,
			)
		}
	}
}

func readAttestationMessages(ctx context.Context, sub *pubsub.Subscription, topic *pubsub.Topic, out chan<- *types.SignedAttestation) {
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

		select {
		case out <- att:
		default:
			gossipLog.Warn("attestation processing channel full, dropping message",
				"slot", att.Message.Slot,
			)
		}
	}
}

func readAggregatedAttestationMessages(ctx context.Context, sub *pubsub.Subscription, out chan<- *types.SignedAggregatedAttestation) {
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

		select {
		case out <- agg:
		default:
			gossipLog.Warn("aggregation processing channel full, dropping message",
				"slot", agg.Data.Slot,
			)
		}
	}
}

func processBlocks(ctx context.Context, ch <-chan *types.SignedBlockWithAttestation, handler *GossipHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-ch:
			if handler.OnBlock != nil {
				handler.OnBlock(block)
			}
		}
	}
}

func processAttestations(ctx context.Context, ch <-chan *types.SignedAttestation, handler *GossipHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case att := <-ch:
			if handler.OnAttestation != nil {
				handler.OnAttestation(att)
			}
		}
	}
}

func processAggregatedAttestations(ctx context.Context, ch <-chan *types.SignedAggregatedAttestation, handler *GossipHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case agg := <-ch:
			if handler.OnAggregatedAttestation != nil {
				handler.OnAggregatedAttestation(agg)
			}
		}
	}
}
