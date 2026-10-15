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

// Verdict is the node's decision on a received gossip message, made before
// pubsub relays it.
type Verdict int

const (
	// Accept delivers the message to the node and relays it.
	Accept Verdict = iota
	// Ignore drops the message without blaming the sender: it may be valid to
	// a peer with a different view, or later.
	Ignore
	// Reject drops the message as invalid for every node.
	Reject
)

type MessageHandler interface {
	OnBlock(block *types.SignedBlock)
	OnGossipAttestation(att *types.SignedAttestation)
	OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation)

	// The Validate methods run on pubsub's validation goroutines, concurrently
	// with each other and with the node's own loop.
	ValidateGossipBlock(block *types.SignedBlock) Verdict
	ValidateGossipAttestation(att *types.SignedAttestation) Verdict
	ValidateGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) Verdict
}

func (h *Host) StartGossipListeners(handler MessageHandler) {
	if handler == nil {
		return
	}
	h.gossipHandler.Store(&handler)
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

		switch value := msg.ValidatorData.(type) {
		case *types.SignedBlock:
			blockRoot, err := value.Block.HashTreeRoot()
			if err != nil {
				logger.Error(logger.Gossip, "block root on %s: %v", topic, err)
				continue
			}
			logger.Info(logger.Gossip, "received block slot=%d proposer=%d block_root=0x%x parent_root=0x%x",
				value.Block.Slot, value.Block.ProposerIndex, blockRoot, value.Block.ParentRoot)
			handler.OnBlock(value)
		case *types.SignedAttestation:
			handler.OnGossipAttestation(value)
		case *types.SignedAggregatedAttestation:
			handler.OnGossipAggregatedAttestation(value)
		}
	}
}

// validateGossip decodes a received message and asks the node whether to relay
// it. The decoded value travels to listenTopic in ValidatorData, so each message
// is decoded once. Until the node has started its listeners nothing can be
// judged, so nothing is relayed.
func (h *Host) validateGossip(_ context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
	if from == h.host.ID() {
		return pubsub.ValidationAccept
	}
	handlerPtr := h.gossipHandler.Load()
	if handlerPtr == nil {
		return pubsub.ValidationIgnore
	}
	handler := *handlerPtr

	topic := msg.GetTopic()
	value, verdict, err := h.decodeAndValidate(topic, msg.Data, handler)
	if err != nil {
		logger.Warn(logger.Gossip, "rejecting gossip on %s from %s: %v", topic, from, err)
		return pubsub.ValidationReject
	}
	switch verdict {
	case Accept:
		msg.ValidatorData = value
		return pubsub.ValidationAccept
	case Ignore:
		return pubsub.ValidationIgnore
	default:
		return pubsub.ValidationReject
	}
}

// decodeAndValidate returns an error for a message no node could decode, and
// otherwise the decoded value with the node's verdict on it.
func (h *Host) decodeAndValidate(topic string, raw []byte, handler MessageHandler) (any, Verdict, error) {
	data, err := SnappyRawDecode(raw)
	if err != nil {
		return nil, Reject, fmt.Errorf("snappy decode: %w", err)
	}

	if topic == BlockTopic() {
		if h.Hooks.GossipBlockSize != nil {
			h.Hooks.GossipBlockSize(len(data))
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(data); err != nil {
			return nil, Reject, fmt.Errorf("unmarshal block (%d bytes): %w", len(data), err)
		}
		return block, handler.ValidateGossipBlock(block), nil
	}

	if subnet, ok := attestationSubnet(topic); ok {
		if h.Hooks.GossipAttestationSize != nil {
			h.Hooks.GossipAttestationSize(len(data))
		}
		att := &types.SignedAttestation{}
		if err := att.UnmarshalSSZ(data); err != nil {
			return nil, Reject, fmt.Errorf("unmarshal attestation (%d bytes): %w", len(data), err)
		}
		if want := SubnetID(att.ValidatorID, h.committeeCount); want != subnet {
			return nil, Reject, fmt.Errorf("validator %d belongs on subnet %d, not %d", att.ValidatorID, want, subnet)
		}
		return att, handler.ValidateGossipAttestation(att), nil
	}

	if topic == AggregationTopic() {
		if h.Hooks.GossipAggregationSize != nil {
			h.Hooks.GossipAggregationSize(len(data))
		}
		agg := &types.SignedAggregatedAttestation{}
		if err := agg.UnmarshalSSZ(data); err != nil {
			return nil, Reject, fmt.Errorf("unmarshal aggregation (%d bytes): %w", len(data), err)
		}
		return agg, handler.ValidateGossipAggregatedAttestation(agg), nil
	}

	return nil, Reject, fmt.Errorf("unknown topic: %s", topic)
}
