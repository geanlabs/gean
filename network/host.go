package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	btcec "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
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

// ConnectBootnodes dials the given addresses (multiaddr or ENR) and connects to them.
func ConnectBootnodes(ctx context.Context, h host.Host, addrs []string) {
	for _, addr := range addrs {
		pi, err := parseBootnode(addr)
		if err != nil {
			netLog.Warn("invalid bootnode", "addr", addr, "err", err)
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
}

func parseBootnode(addr string) (*peer.AddrInfo, error) {
	if strings.HasPrefix(addr, "enr:") {
		return p2p.ENRToAddrInfo(addr)
	}
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return nil, err
	}
	return peer.AddrInfoFromP2pAddr(ma)
}

func loadOrGenerateKey(path string) (crypto.PrivKey, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			// Try hex-encoded secp256k1 key first (lean-quickstart format).
			if key, hexErr := parseHexKey(strings.TrimSpace(string(data))); hexErr == nil {
				return key, nil
			}
			// Fall back to libp2p protobuf-marshaled key.
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

// parseHexKey decodes a hex-encoded 32-byte secp256k1 private key.
func parseHexKey(s string) (crypto.PrivKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("expected 32-byte key, got %d", len(b))
	}
	secKey := btcec.PrivKeyFromBytes(b)
	return (*crypto.Secp256k1PrivateKey)(secKey), nil
}
