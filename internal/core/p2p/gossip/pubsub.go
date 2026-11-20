// Package gossip is the gossipsub layer shared by consensus networks: the
// mesh parameters, the message-id function and the snappy block encoding of
// gossip payloads.
package gossip

import (
	"context"
	"fmt"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2phost "github.com/libp2p/go-libp2p/core/host"

	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
)

const (
	MeshN             = 8
	MeshNLow          = 6
	MeshNHigh         = 12
	Lazy              = 6
	HeartbeatInterval = 700 * time.Millisecond
	FanoutTTL         = 60 * time.Second
	HistoryLength     = 6
	HistoryGossip     = 3
	DuplicateCache    = 24 * time.Second
	MaxTransmitSize   = reqresp.MaxCompressedPayloadSize
	MaxMsgPerRPC      = 500
	// ValidateQueueSize bounds messages waiting for validation. pubsub's
	// default of 32 drops anything beyond one small burst, silently; this holds
	// several full RPCs.
	ValidateQueueSize = 4 * MaxMsgPerRPC
)

// NewPubSub starts gossipsub on h with the mesh parameters above.
func NewPubSub(ctx context.Context, h libp2phost.Host) (*pubsub.PubSub, error) {
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageIdFn(func(msg *pb.Message) string {
			topic := ""
			if msg.Topic != nil {
				topic = *msg.Topic
			}
			return string(MessageID(topic, msg.Data))
		}),
		pubsub.WithGossipSubParams(gossipSubParams()),
		pubsub.WithSeenMessagesTTL(DuplicateCache),
		pubsub.WithMaxMessageSize(MaxTransmitSize),
		pubsub.WithValidateQueueSize(ValidateQueueSize),
	)
	if err != nil {
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}
	return ps, nil
}

func gossipSubParams() pubsub.GossipSubParams {
	params := pubsub.DefaultGossipSubParams()
	params.D = MeshN
	params.Dlo = MeshNLow
	params.Dhi = MeshNHigh
	params.Dlazy = Lazy
	params.HeartbeatInterval = HeartbeatInterval
	params.FanoutTTL = FanoutTTL
	params.HistoryLength = HistoryLength
	params.HistoryGossip = HistoryGossip
	params.MaxIHaveMessages = MaxMsgPerRPC
	return params
}
