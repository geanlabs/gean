package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/p2p"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
)

var netLog = logging.NewComponentLogger(logging.CompNetwork)

// ErrUnsupportedKeyFormat is returned when a node key file cannot be parsed.
var ErrUnsupportedKeyFormat = errors.New("unsupported key format")

// allowAllGater is a connection gater that allows all connections (devnet: no filtering).
type allowAllGater struct{}

func (g *allowAllGater) InterceptPeerDial(p peer.ID) bool                         { return true }
func (g *allowAllGater) InterceptAddrDial(id peer.ID, m multiaddr.Multiaddr) bool { return true }
func (g *allowAllGater) InterceptAccept(cm libp2pnetwork.ConnMultiaddrs) bool     { return true }
func (g *allowAllGater) InterceptSecured(d libp2pnetwork.Direction, id peer.ID, cm libp2pnetwork.ConnMultiaddrs) bool {
	return true
}
func (g *allowAllGater) InterceptUpgraded(c libp2pnetwork.Conn) (bool, control.DisconnectReason) {
	return true, 0
}

const nodeKeyFilePerms = 0600

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

	// Configure resource manager with no limits (for devnet compatibility)
	rmgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create resource manager: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(addr),
		libp2p.DefaultTransports,
		libp2p.ResourceManager(rmgr),
		libp2p.ConnectionGater(&allowAllGater{}),
		libp2p.DisableRelay(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("new host: %w", err)
	}

	// Log actual listen addresses for debugging
	for _, a := range h.Addrs() {
		netLog.Info("listening on", "addr", a.String())
	}

	var directPeers []peer.AddrInfo
	for _, addr := range bootnodes {
		pi, err := parseBootnode(addr)
		if err != nil || pi.ID == h.ID() {
			continue
		}
		directPeers = append(directPeers, *pi)
	}

	gs, err := gossipsub.NewGossipSub(ctx, h, directPeers)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("gossipsub: %w", err)
	}

	// Register peer connection/disconnection notification handler for metrics.
	h.Network().Notify(&libp2pnetwork.NotifyBundle{
		ConnectedF: func(n libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			dir := "inbound"
			if conn.Stat().Direction == libp2pnetwork.DirOutbound {
				dir = "outbound"
			}
			metrics.PeerConnectionEventsTotal.WithLabelValues(dir, "success").Inc()
		},
		DisconnectedF: func(n libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			dir := "inbound"
			if conn.Stat().Direction == libp2pnetwork.DirOutbound {
				dir = "outbound"
			}
			metrics.PeerDisconnectionEventsTotal.WithLabelValues(dir, "remote_close").Inc()
		},
	})

	return &Host{P2P: h, PubSub: gs, Ctx: ctx, Cancel: cancel}, nil
}

// Close shuts down the host.
func (h *Host) Close() error {
	h.Cancel()
	return h.P2P.Close()
}

// ConnectBootnodes dials the given addresses (multiaddr or ENR) and connects to them sequentially.
func ConnectBootnodes(ctx context.Context, h host.Host, addrs []string) {
	for _, addr := range addrs {
		pi, err := parseBootnode(addr)
		if err != nil {
			netLog.Warn("invalid bootnode", "addr", addr, "err", err)
			continue
		}
		if pi.ID == h.ID() {
			continue
		}

		if err := h.Connect(ctx, *pi); err != nil {
			result := "error"
			if ctx.Err() != nil {
				result = "timeout"
			}
			metrics.PeerConnectionEventsTotal.WithLabelValues("outbound", result).Inc()
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

// loadOrGenerateKey tries to read a node identity key from disk, or generates
// and saves a new one if it does not exist.
func loadOrGenerateKey(path string) (crypto.PrivKey, error) {
	if path == "" {
		priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
		return priv, err
	}

	if _, err := os.Stat(path); err == nil {
		return loadKey(path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat key file: %w", err)
	}

	return generateAndSaveKey(path)
}

// loadKey reads an existing key from disk, attempting to decode it as protobuf
// or raw hex.
func loadKey(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// Try protobuf format first (native libp2p format).
	if priv, err := crypto.UnmarshalPrivateKey(data); err == nil {
		return priv, nil
	}

	// Fall back to hex-encoded raw secp256k1 key (generated by generate-genesis.sh).
	hexStr := strings.TrimSpace(string(data))
	raw, hexErr := hex.DecodeString(hexStr)
	if hexErr == nil && len(raw) == 32 {
		priv, err := crypto.UnmarshalSecp256k1PrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal hex key: %w", err)
		}
		return priv, nil
	}

	return nil, fmt.Errorf("%w in %s", ErrUnsupportedKeyFormat, path)
}

// generateAndSaveKey creates a new secp256k1 private key and writes it
// safely to disk.
func generateAndSaveKey(path string) (crypto.PrivKey, error) {
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate secp256k1 key: %w", err)
	}

	raw, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	if writeErr := os.WriteFile(path, raw, nodeKeyFilePerms); writeErr != nil {
		return nil, fmt.Errorf("save key: %w", writeErr)
	}

	return priv, nil
}
