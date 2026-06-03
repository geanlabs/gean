package p2p

import (
	"context"
	"errors"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

type MessageHandler interface {
	OnBlock(block *types.SignedBlock)
	OnGossipAttestation(att *types.SignedAttestation)
	OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation)
}

func (h *Host) StartGossipListeners(handler MessageHandler) {
	if handler == nil {
		return
	}
	h.gossipHandler = handler
	for topic, sub := range h.subs {
		go h.listenTopic(h.ctx, topic, sub, handler)
	}
}

func (h *Host) listenTopic(ctx context.Context, topic string, sub *pubsub.Subscription, handler MessageHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, pubsub.ErrSubscriptionCancelled) {
				return
			}
			logger.Error(logger.Gossip, "recv error on %s: %v", topic, err)
			return
		}

		if msg.ReceivedFrom == h.host.ID() {
			continue
		}

		data, err := SnappyRawDecode(msg.Data)
		if err != nil {
			logger.Error(logger.Gossip, "snappy decode failed on %s: %v", topic, err)
			continue
		}

		if err := h.dispatchMessage(topic, data, handler); err != nil {
			logger.Error(logger.Gossip, "dispatch failed on %s: %v", topic, err)
		}
	}
}

func (h *Host) dispatchMessage(topic string, data []byte, handler MessageHandler) error {
	if handler == nil {
		return fmt.Errorf("missing gossip handler")
	}

	switch {
	case topic == BlockTopic():
		if h.Hooks.GossipBlockSize != nil {
			h.Hooks.GossipBlockSize(len(data))
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return fmt.Errorf("unmarshal block (%d bytes): %w", len(data), err)
		}
		if block.Block == nil {
			return fmt.Errorf("malformed block: missing block")
		}
		blockRoot, err := block.Block.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("block root: %w", err)
		}
		logger.Info(logger.Gossip, "received block slot=%d proposer=%d block_root=0x%x parent_root=0x%x",
			block.Block.Slot, block.Block.ProposerIndex,
			blockRoot, block.Block.ParentRoot)
		handler.OnBlock(block)

	case isAttestationSubnetTopic(topic):
		if h.Hooks.GossipAttestationSize != nil {
			h.Hooks.GossipAttestationSize(len(data))
		}
		att := &types.SignedAttestation{}
		if err := att.UnmarshalSSZ(data); err != nil {
			return fmt.Errorf("unmarshal attestation (%d bytes): %w", len(data), err)
		}
		handler.OnGossipAttestation(att)

	case topic == AggregationTopic():
		if h.Hooks.GossipAggregationSize != nil {
			h.Hooks.GossipAggregationSize(len(data))
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
