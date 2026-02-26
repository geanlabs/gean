package gossipsub

import (
	"context"

	"github.com/golang/snappy"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/types"
)

// GossipHandler processes decoded gossip messages.
type GossipHandler struct {
	OnBlock       func(*types.SignedBlockWithAttestation)
	OnAttestation func(*types.SignedAttestation)
}

// SubscribeTopics subscribes to topics and dispatches messages to handler.
func SubscribeTopics(ctx context.Context, topics *Topics, handler *GossipHandler) error {
	blockSub, err := topics.Block.Subscribe()
	if err != nil {
		return err
	}
	attSub, err := topics.Attestation.Subscribe()
	if err != nil {
		return err
	}

	go readBlockMessages(ctx, blockSub, handler)
	go readAttestationMessages(ctx, attSub, handler)
	return nil
}

func readBlockMessages(ctx context.Context, sub *pubsub.Subscription, handler *GossipHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		decoded, err := snappy.Decode(nil, msg.Data)
		if err != nil {
			continue
		}
		block := new(types.SignedBlockWithAttestation)
		if err := block.UnmarshalSSZ(decoded); err != nil {
			continue
		}
		if handler.OnBlock != nil {
			handler.OnBlock(block)
		}
	}
}

func readAttestationMessages(ctx context.Context, sub *pubsub.Subscription, handler *GossipHandler) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
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
