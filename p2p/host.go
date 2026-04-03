package p2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/logger"
)

// GossipSub parameters matching ethlambda p2p/lib.rs L96-119.
const (
	GossipMeshN             = 8
	GossipMeshNLow          = 6
	GossipMeshNHigh         = 12
	GossipLazy              = 6
	GossipHeartbeatInterval = 700 * time.Millisecond
	GossipFanoutTTL         = 60 * time.Second
	GossipHistoryLength     = 6
	GossipHistoryGossip     = 3
	GossipDuplicateCache    = 24 * time.Second // 4s slot * 3 lookback * 2
	GossipMaxTransmitSize   = MaxCompressedPayloadSize
	GossipMaxMsgPerRPC      = 500
)

// Host wraps a libp2p host with gossipsub and topic handles.
type Host struct {
	host       host.Host
	pubsub     *pubsub.PubSub
	topics     map[string]*pubsub.Topic
	subs       map[string]*pubsub.Subscription
	ctx        context.Context
	cancel     context.CancelFunc
	peerStore  *PeerStore
	listenPort int
}

// NewHost creates a libp2p host with QUIC transport and gossipsub.
// Matches ethlambda p2p/lib.rs swarm initialization (L60-160).
func NewHost(ctx context.Context, nodeKeyPath string, listenPort int, committeeCount uint64) (*Host, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Load secp256k1 identity from hex-encoded node key file.
	privKey, err := loadNodeKey(nodeKeyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load node key: %w", err)
	}

	// Create libp2p host with QUIC transport.
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", listenPort))

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddr),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.DisableRelay(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	// Create gossipsub with custom parameters.
	// Anonymous message signing matches ethlambda ValidationMode::Anonymous
	// + MessageAuthenticity::Anonymous — lean consensus messages have no signature.
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
		pubsub.WithGossipSubParams(func() pubsub.GossipSubParams {
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
		}()),
		pubsub.WithSeenMessagesTTL(GossipDuplicateCache),
		pubsub.WithMaxMessageSize(GossipMaxTransmitSize),
	)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	p2pHost := &Host{
		host:       h,
		pubsub:     ps,
		topics:     make(map[string]*pubsub.Topic),
		subs:       make(map[string]*pubsub.Subscription),
		ctx:        ctx,
		cancel:     cancel,
		peerStore:  NewPeerStore(),
		listenPort: listenPort,
	}

	// Register connection notifier to track peers.
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			p2pHost.peerStore.Add(peerID)
			logger.Info(logger.Network, "peer connected peer_id=%s direction=%s peers=%d",
				peerID, conn.Stat().Direction, p2pHost.peerStore.Count())
		},
		DisconnectedF: func(n network.Network, conn network.Conn) {
			peerID := conn.RemotePeer()
			// Only remove if fully disconnected (no remaining connections).
			if n.Connectedness(peerID) != network.Connected {
				p2pHost.peerStore.Remove(peerID)
				logger.Info(logger.Network, "peer disconnected peer_id=%s peers=%d",
					peerID, p2pHost.peerStore.Count())
			}
		},
	})

	// Join default topics.
	logger.Info(logger.Network, "joining gossipsub topics")
	if err := p2pHost.JoinTopic(BlockTopic()); err != nil {
		p2pHost.Close()
		return nil, fmt.Errorf("join block topic: %w", err)
	}
	if err := p2pHost.JoinTopic(AggregationTopic()); err != nil {
		p2pHost.Close()
		return nil, fmt.Errorf("join aggregation topic: %w", err)
	}

	// Join attestation subnet topics.
	for i := uint64(0); i < committeeCount; i++ {
		if err := p2pHost.JoinTopic(AttestationSubnetTopic(i)); err != nil {
			p2pHost.Close()
			return nil, fmt.Errorf("join attestation subnet %d: %w", i, err)
		}
	}

	// Log all subscribed topics.
	for topic := range p2pHost.topics {
		logger.Info(logger.Network, "subscribed topic=%s", topic)
	}

	return p2pHost, nil
}

// JoinTopic joins a gossipsub topic and subscribes to it.
func (h *Host) JoinTopic(topic string) error {
	t, err := h.pubsub.Join(topic)
	if err != nil {
		return fmt.Errorf("join topic %s: %w", topic, err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}
	h.topics[topic] = t
	h.subs[topic] = sub
	return nil
}

// PeerID returns this host's peer ID.
func (h *Host) PeerID() peer.ID {
	return h.host.ID()
}

// Addrs returns the host's listen addresses.
func (h *Host) Addrs() []multiaddr.Multiaddr {
	return h.host.Addrs()
}

// ConnectedPeers returns the number of connected peers.
func (h *Host) ConnectedPeers() int {
	return h.peerStore.Count()
}

// TopicMeshSizes returns a map of topic name to mesh peer count.
func (h *Host) TopicMeshSizes() map[string]int {
	sizes := make(map[string]int)
	for name, topic := range h.topics {
		sizes[name] = len(topic.ListPeers())
	}
	return sizes
}

// LibP2PHost returns the underlying libp2p host for req-resp stream handlers.
func (h *Host) LibP2PHost() host.Host {
	return h.host
}

// Close shuts down the host.
func (h *Host) Close() {
	h.cancel()
	for _, sub := range h.subs {
		sub.Cancel()
	}
	h.host.Close()
}

// loadNodeKey reads a hex-encoded secp256k1 private key from a file.
// Matches ethlambda main.rs read_hex_file_bytes + secp256k1 conversion.
func loadNodeKey(path string) (libp2pcrypto.PrivKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	hexStr := strings.TrimSpace(string(content))
	hexStr = strings.TrimPrefix(hexStr, "0x")

	keyBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}

	privKey, err := libp2pcrypto.UnmarshalSecp256k1PrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse secp256k1 key: %w", err)
	}

	return privKey, nil
}

// ConnectPeer connects to a peer at the given multiaddr.
func (h *Host) ConnectPeer(ctx context.Context, addr multiaddr.Multiaddr) error {
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("parse peer addr: %w", err)
	}
	return h.host.Connect(ctx, *peerInfo)
}
