package p2p

import (
	"context"
	"errors"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

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

		switch m := msg.ValidatorData.(type) {
		case *types.SignedBlock:
			blockRoot, err := m.Block.HashTreeRoot()
			if err != nil {
				logger.Error(logger.Gossip, "block root on %s: %v", topic, err)
				continue
			}
			logger.Info(logger.Gossip, "received block slot=%d proposer=%d block_root=0x%x parent_root=0x%x",
				m.Block.Slot, m.Block.ProposerIndex, blockRoot, m.Block.ParentRoot)
			handler.OnBlock(m)
		case *types.SignedAttestation:
			handler.OnGossipAttestation(m)
		case *types.SignedAggregatedAttestation:
			handler.OnGossipAggregatedAttestation(m)
		}
	}
}

// validateGossip decodes a remote message so that only decodable messages are
// relayed; the decoded value reaches listenTopic as the message's ValidatorData.
func (h *Host) validateGossip(_ context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if from == h.host.ID() {
		return pubsub.ValidationAccept
	}
	decoded, err := h.decodeGossip(msg.GetTopic(), msg.Data)
	if err != nil {
		logger.Warn(logger.Gossip, "rejected message on %s from %s: %v", msg.GetTopic(), from, err)
		return pubsub.ValidationReject
	}
	msg.ValidatorData = decoded
	return pubsub.ValidationAccept
}

func (h *Host) decodeGossip(topic string, compressed []byte) (any, error) {
	data, err := SnappyRawDecode(compressed)
	if err != nil {
		return nil, fmt.Errorf("snappy decode: %w", err)
	}

	switch {
	case topic == BlockTopic():
		if h.Hooks.GossipBlockSize != nil {
			h.Hooks.GossipBlockSize(len(data))
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return nil, fmt.Errorf("unmarshal block (%d bytes): %w", len(data), err)
		}
		if block.Block == nil {
			return nil, fmt.Errorf("malformed block: missing block")
		}
		// The decoder also accepts an empty attestation list padded to four
		// bytes. That encoding has the same root but another message ID, so
		// relaying it would let a block bypass gossip deduplication.
		if size := block.SizeSSZ(); size != len(data) {
			return nil, fmt.Errorf("non-canonical block: %d bytes, canonical %d", len(data), size)
		}
		return block, nil

	case isAttestationSubnetTopic(topic):
		if h.Hooks.GossipAttestationSize != nil {
			h.Hooks.GossipAttestationSize(len(data))
		}
		att := &types.SignedAttestation{}
		if err := att.UnmarshalSSZ(data); err != nil {
			return nil, fmt.Errorf("unmarshal attestation (%d bytes): %w", len(data), err)
		}
		return att, nil

	case topic == AggregationTopic():
		if h.Hooks.GossipAggregationSize != nil {
			h.Hooks.GossipAggregationSize(len(data))
		}
		agg := &types.SignedAggregatedAttestation{}
		if err := agg.UnmarshalSSZ(data); err != nil {
			return nil, fmt.Errorf("unmarshal aggregation (%d bytes): %w", len(data), err)
		}
		return agg, nil

	default:
		return nil, fmt.Errorf("unknown topic: %s", topic)
	}
}
