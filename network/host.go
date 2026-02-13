package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
)

var netLog = logging.NewComponentLogger(logging.CompNetwork)

// Host wraps a libp2p host with gossipsub and protocol handlers.
type Host struct {
	P2P    host.Host
	PubSub *pubsub.PubSub
	Ctx    context.Context
	Cancel context.CancelFunc
}

// NewHost creates a libp2p host with QUIC transport and secp256k1 identity.
func NewHost(listenAddr string, nodeKeyPath string, bootnodes []string) (*Host, error) {
	ctx, cancel := context.WithCancel(context.Background())

	privKey, err := loadOrGenerateKey(nodeKeyPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load key: %w", err)
	}

	addr, err := multiaddr.NewMultiaddr(listenAddr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("parse listen addr: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(addr),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("new host: %w", err)
	}

	gs, err := gossipsub.NewGossipSub(ctx, h)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("gossipsub: %w", err)
	}

	return &Host{P2P: h, PubSub: gs, Ctx: ctx, Cancel: cancel}, nil
}

// Close shuts down the host.
func (h *Host) Close() error {
	h.Cancel()
	return h.P2P.Close()
}

// ConnectBootnodes dials the given multiaddrs and connects to them.
func ConnectBootnodes(ctx context.Context, h host.Host, addrs []string) error {
	for _, addr := range addrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			netLog.Warn("invalid bootnode multiaddr", "addr", addr, "err", err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			netLog.Warn("invalid bootnode peer info", "addr", addr, "err", err)
			continue
		}
		if pi.ID == h.ID() {
			continue // skip self
		}
		if err := h.Connect(ctx, *pi); err != nil {
			netLog.Warn("failed to connect to bootnode",
				"peer_id", pi.ID.String()[:16]+"...",
				"err", err,
			)
			continue
		}
		netLog.Info("connected to bootnode",
			"peer_id", pi.ID.String()[:16]+"...",
		)
	}
	return nil
}

func loadOrGenerateKey(path string) (crypto.PrivKey, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			return crypto.UnmarshalPrivateKey(data)
		}
		// File doesn't exist — generate and save.
		priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
		if err != nil {
			return nil, err
		}
		raw, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		if writeErr := os.WriteFile(path, raw, 0600); writeErr != nil {
			return nil, fmt.Errorf("save key: %w", writeErr)
		}
		return priv, nil
	}
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	return priv, err
}
