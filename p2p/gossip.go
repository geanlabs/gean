package p2p

import (
	"context"
	"fmt"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// MessageHandler defines callbacks for gossipsub messages.
// Engine implements this interface and processes messages on its own goroutine.
type MessageHandler interface {
	OnBlock(block *types.SignedBlock)
	OnGossipAttestation(att *types.SignedAttestation)
	OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation)
}

// Gossip-size metric hooks set by node package at startup. Nil-safe.
var (
	GossipBlockSizeHook       func(bytes int)
	GossipAttestationSizeHook func(bytes int)
	GossipAggregationSizeHook func(bytes int)
)

// StartGossipListeners starts goroutines that read from each subscribed topic
// and dispatch decoded messages to the handler.
func (h *Host) StartGossipListeners(handler MessageHandler) {
	for topic, sub := range h.subs {
		go h.listenTopic(h.ctx, topic, sub, handler)
	}
}

func (h *Host) listenTopic(ctx context.Context, topic string, sub *pubsub.Subscription, handler MessageHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled, clean shutdown
			}
			logger.Error(logger.Gossip, "recv error on %s: %v", topic, err)
			return
		}

		// Skip messages from ourselves.
		if msg.ReceivedFrom == h.host.ID() {
			continue
		}

		// Decompress raw snappy.
		data, err := SnappyRawDecode(msg.Data)
		if err != nil {
			logger.Error(logger.Gossip, "snappy decode failed on %s: %v", topic, err)
			continue
		}

		// Dispatch based on topic kind.
		if err := h.dispatchMessage(topic, data, handler); err != nil {
			logger.Error(logger.Gossip, "dispatch failed on %s: %v", topic, err)
		}
	}
}

func (h *Host) dispatchMessage(topic string, data []byte, handler MessageHandler) error {
	switch {
	case topic == BlockTopic():
		if GossipBlockSizeHook != nil {
			GossipBlockSizeHook(len(data))
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return fmt.Errorf("unmarshal block (%d bytes): %w", len(data), err)
		}
		blockRoot, _ := block.Block.HashTreeRoot()
		logger.Info(logger.Gossip, "received block slot=%d proposer=%d block_root=0x%x parent_root=0x%x",
			block.Block.Slot, block.Block.ProposerIndex,
			blockRoot, block.Block.ParentRoot)
		handler.OnBlock(block)

	case strings.Contains(topic, AttestationTopicKind+"_"):
		if GossipAttestationSizeHook != nil {
			GossipAttestationSizeHook(len(data))
		}
		att := &types.SignedAttestation{}
		if err := att.UnmarshalSSZ(data); err != nil {
			return fmt.Errorf("unmarshal attestation (%d bytes): %w", len(data), err)
		}
		handler.OnGossipAttestation(att)

	case topic == AggregationTopic():
		if GossipAggregationSizeHook != nil {
			GossipAggregationSizeHook(len(data))
		}
		agg := &types.SignedAggregatedAttestation{}
		if err := agg.UnmarshalSSZ(data); err != nil {
			return fmt.Errorf("unmarshal aggregation (%d bytes): %w", len(data), err)
		}
		handler.OnGossipAggregatedAttestation(agg)

	default:
		return fmt.Errorf("unknown topic: %s", topic)
	}

	return nil
}
