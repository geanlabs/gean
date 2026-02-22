package network

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"time"

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

// ConnectBootnodes dials the given addresses once. Returns after attempting all.
func ConnectBootnodes(ctx context.Context, h host.Host, addrs []string) error {
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
	return nil
}

// ConnectBootnodesWithRetry dials bootnodes immediately and then keeps
// re-dialing any that are not currently connected, with exponential backoff
// up to maxInterval. It runs until ctx is cancelled.
func ConnectBootnodesWithRetry(ctx context.Context, h host.Host, addrs []string) {
	infos := make([]*peer.AddrInfo, 0, len(addrs))
	for _, addr := range addrs {
		pi, err := parseBootnode(addr)
		if err != nil {
			netLog.Warn("invalid bootnode", "addr", addr, "err", err)
			continue
		}
		if pi.ID == h.ID() {
			continue
		}
		infos = append(infos, pi)
	}

	if len(infos) == 0 {
		return
	}

	// Initial connect attempt.
	connectAll(ctx, h, infos)

	go func() {
		const (
			minInterval = 5 * time.Second
			maxInterval = 60 * time.Second
		)
		interval := minInterval

		t := time.NewTimer(interval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				connected := connectAll(ctx, h, infos)
				if connected == len(infos) {
					// All connected — slow down polling.
					interval = maxInterval
				} else {
					// Some missing — back off but not too slow.
					interval = interval * 2
					if interval > maxInterval {
						interval = maxInterval
					}
				}
				t.Reset(interval)
			}
		}
	}()
}

// connectAll tries to connect to each peer that isn't already connected.
// Returns the number of peers that are connected after the attempt.
func connectAll(ctx context.Context, h host.Host, infos []*peer.AddrInfo) int {
	connected := 0
	for _, pi := range infos {
		// Already connected — count it, skip dial.
		if len(h.Network().ConnsToPeer(pi.ID)) > 0 {
			connected++
			continue
		}
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := h.Connect(dialCtx, *pi)
		cancel()
		if err != nil {
			netLog.Warn("failed to connect to bootnode",
				"peer_id", pi.ID.String()[:16]+"...",
				"err", err,
			)
		} else {
			netLog.Info("connected to bootnode",
				"peer_id", pi.ID.String()[:16]+"...",
			)
			connected++
		}
	}
	return connected
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