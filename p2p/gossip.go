package p2p

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"

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

// Peer event metric hooks set by node package at startup. Nil-safe.
// Direction is "inbound" or "outbound" per the libp2p connection stat.
// Reason on disconnect is best-effort; libp2p doesn't always expose a
// precise cause, so callers default to "remote_close" for ordinary
// disconnects. PeerCountHook reports the aggregate connected peer count
// after every connect/disconnect.
var (
	PeerConnectedHook    func(direction string)
	PeerDisconnectedHook func(direction, reason string)
	PeerCountHook        func(count int)

	// PeerStatusHook fires once per newly-connected peer (gated by
	// PeerStore.AddNew so reconnect events do not retrigger) with the peer's
	// ID. Wired by node-layer code to initiate the lean P2P Status reqresp
	// handshake the protocol expects on each new connection. Sending Status
	// from the libp2p connection notifier (rather than only from the sync
	// driver's periodic poll) is required so cold-start single-peer setups
	// learn the peer's head promptly enough to drive backfill.
	PeerStatusHook func(peerID peer.ID)
)

// StartGossipListeners starts goroutines that read from each subscribed topic
// and dispatch decoded messages to the handler. The handler is also retained
// on the Host so that subscriptions replaced later (e.g. by Reannounce-
// Subscriptions, which has to Cancel the existing Subscription to drive the
// 0→1 mySubs transition that triggers a fresh announce RPC) can keep being
// drained by a fresh listenTopic goroutine.
func (h *Host) StartGossipListeners(handler MessageHandler) {
	h.gossipHandler = handler
	for topic, sub := range h.subs {
		go h.listenTopic(h.ctx, topic, sub, handler)
	}
}

func (h *Host) listenTopic(ctx context.Context, topic string, sub *pubsub.Subscription, handler MessageHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			// Clean exits: host shutdown cancels ctx; ReannounceSubscriptions
			// / Close cancel the subscription directly, which surfaces as
			// pubsub.ErrSubscriptionCancelled while ctx is still live.
			if ctx.Err() != nil || errors.Is(err, pubsub.ErrSubscriptionCancelled) {
				return
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
