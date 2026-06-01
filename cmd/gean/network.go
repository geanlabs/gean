package main

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/node"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/syncer"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"github.com/multiformats/go-multiaddr"
)

func setupP2P(ctx context.Context, cfg config, keyManager *xmss.KeyManager) (*p2p.Host, error) {
	var validatorIDs []uint64
	if keyManager != nil {
		validatorIDs = keyManager.ValidatorIDs()
	}

	p2pHost, err := p2p.NewHost(ctx, cfg.NodeKey, cfg.GossipPort, cfg.CommitteeCount, validatorIDs, cfg.IsAggregator, cfg.AggregateSubnetIDs)
	if err != nil {
		return nil, err
	}

	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", p2pHost.PeerID(), cfg.GossipPort)
	return p2pHost, nil
}

func preinitializeXMSS(isAggregator bool) {
	if isAggregator {
		logger.Info(logger.Node, "pre-initializing XMSS prover (this takes ~45s)...")
		xmss.EnsureProverReady()
		logger.Info(logger.Node, "XMSS prover ready")
	}
	xmss.EnsureVerifierReady()
}

func registerReqRespHandlers(p2pHost *p2p.Host, s *store.ConsensusStore) {
	p2pHost.RegisterReqRespHandlers(
		func() *p2p.StatusMessage {
			finalized := s.LatestFinalized()
			return &p2p.StatusMessage{
				FinalizedRoot: finalized.Root,
				FinalizedSlot: finalized.Slot,
				HeadRoot:      s.Head(),
				HeadSlot:      s.HeadSlot(),
			}
		},
		func(root [32]byte) *types.SignedBlock {
			return s.GetSignedBlock(root)
		},
		func() uint64 {
			return s.HeadSlot()
		},
		func(startSlot, count uint64) []*types.SignedBlock {
			return s.GetCanonicalBlocksInRange(startSlot, count)
		},
	)
}

func startNodeNetworking(ctx context.Context, n *node.Engine, s *store.ConsensusStore, p2pHost *p2p.Host, bootnodes []multiaddr.Multiaddr) {
	p2pHost.StartGossipListeners(n)
	go n.Run(ctx)

	// Wire PeerStatusHook BEFORE dialing bootnodes (below). A fast initial
	// dial must not fire a peer-connected event while the hook is still nil,
	// or the Status reqresp handshake that drives backfill is skipped for that
	// peer. Keep these two statements in this order.
	syncDriver := syncer.NewSyncDriver(ctx, n, s, p2pHost)
	p2p.PeerStatusHook = syncDriver.OnPeerConnected
	go syncDriver.Run()

	p2pHost.ConnectBootnodes(ctx, bootnodes)
	p2pHost.StartBootnodeRedial(ctx, bootnodes)
	scheduleSubscriptionReannounce(ctx, p2pHost)
}

func scheduleSubscriptionReannounce(ctx context.Context, p2pHost *p2p.Host) {
	go func() {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
		if err := p2pHost.ReannounceSubscriptions(); err != nil {
			logger.Error(logger.Network, "re-announce subscriptions failed: %v", err)
		}
	}()
}
