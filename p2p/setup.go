package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/multiformats/go-multiaddr"
)

const (
	GossipMeshN             = 8
	GossipMeshNLow          = 6
	GossipMeshNHigh         = 12
	GossipLazy              = 6
	GossipHeartbeatInterval = 700 * time.Millisecond
	GossipFanoutTTL         = 60 * time.Second
	GossipHistoryLength     = 6
	GossipHistoryGossip     = 3
	GossipDuplicateCache    = 24 * time.Second
	GossipMaxTransmitSize   = MaxCompressedPayloadSize
	GossipMaxMsgPerRPC      = 500
)

func newLibP2PHost(privKey libp2pcrypto.PrivKey, listenPort int) (libp2phost.Host, error) {
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", listenPort))
	if err != nil {
		return nil, fmt.Errorf("build listen addr: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddr),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.DisableRelay(),
	)
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}
	return h, nil
}

func newGossipSub(ctx context.Context, h libp2phost.Host) (*pubsub.PubSub, error) {
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageIdFn(func(msg *pb.Message) string {
			topic := ""
			if msg.Topic != nil {
				topic = *msg.Topic
			}
			return string(ComputeMessageID(topic, msg.Data))
		}),
		pubsub.WithGossipSubParams(gossipSubParams()),
		pubsub.WithSeenMessagesTTL(GossipDuplicateCache),
		pubsub.WithMaxMessageSize(GossipMaxTransmitSize),
	)
	if err != nil {
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}
	return ps, nil
}

func gossipSubParams() pubsub.GossipSubParams {
	params := pubsub.DefaultGossipSubParams()
	params.D = GossipMeshN
	params.Dlo = GossipMeshNLow
	params.Dhi = GossipMeshNHigh
	params.Dlazy = GossipLazy
	params.HeartbeatInterval = GossipHeartbeatInterval
	params.FanoutTTL = GossipFanoutTTL
	params.HistoryLength = GossipHistoryLength
	params.HistoryGossip = GossipHistoryGossip
	params.MaxIHaveMessages = GossipMaxMsgPerRPC
	return params
}
