package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/internal/logger"
)

const BootnodeRedialSecs = 12

func (h *Host) ConnectBootnodes(ctx context.Context, addrs []multiaddr.Multiaddr) {
	for _, addr := range addrs {
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			logger.Warn(logger.Network, "invalid bootnode addr %s: %v", addr, err)
			continue
		}
		if err := h.host.Connect(ctx, *peerInfo); err != nil {
			logger.Warn(logger.Network, "bootnode connect failed %s: %v", addr, err)
			continue
		}
		logger.Info(logger.Network, "connected to bootnode %s", peerInfo.ID.ShortString())
	}
}

func (h *Host) StartBootnodeRedial(ctx context.Context, addrs []multiaddr.Multiaddr) {
	go func() {
		ticker := time.NewTicker(BootnodeRedialSecs * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.redialBootnodes(ctx, addrs)
			}
		}
	}()
}

func (h *Host) redialBootnodes(ctx context.Context, addrs []multiaddr.Multiaddr) {
	for _, addr := range addrs {
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		if h.host.Network().Connectedness(peerInfo.ID) == network.Connected {
			continue
		}
		if err := h.host.Connect(ctx, *peerInfo); err == nil {
			logger.Info(logger.Network, "reconnected to bootnode %s", peerInfo.ID.ShortString())
		}
	}
}

func (h *Host) ConnectPeer(ctx context.Context, addr multiaddr.Multiaddr) error {
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("parse peer addr: %w", err)
	}
	return h.host.Connect(ctx, *peerInfo)
}
