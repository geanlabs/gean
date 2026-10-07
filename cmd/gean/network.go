package main

import (
	"context"
	"time"

	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/syncer"
	"github.com/geanlabs/gean/tasks"
	"github.com/geanlabs/gean/types"
	"github.com/multiformats/go-multiaddr"
)

func setupP2P(ctx context.Context, cfg config, keyManager *xmss.KeyManager, m *metrics.Metrics) (*p2p.Host, error) {
	var validatorIDs []uint64
	if keyManager != nil {
		validatorIDs = keyManager.ValidatorIDs()
	}

	p2pHost, err := p2p.NewHost(ctx, cfg.NodeKey, cfg.GossipPort, cfg.CommitteeCount, validatorIDs, cfg.IsAggregator, cfg.AggregateSubnetIDs)
	if err != nil {
		return nil, err
	}

	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", p2pHost.PeerID(), cfg.GossipPort)
	instrumentP2P(p2pHost, m)
	return p2pHost, nil
}

func instrumentP2P(p2pHost *p2p.Host, m *metrics.Metrics) {
	p2pHost.Hooks.GossipBlockSize = m.ObserveGossipBlockSize
	p2pHost.Hooks.GossipAttestationSize = m.ObserveGossipAttestationSize
	p2pHost.Hooks.GossipAggregationSize = m.ObserveGossipAggregationSize
	p2pHost.Hooks.PeerConnected = func(direction string) {
		m.IncPeerConnection(direction, "success")
	}
	p2pHost.Hooks.PeerDisconnected = func(direction, reason string) {
		m.IncPeerDisconnection(direction, reason)
	}
	p2pHost.Hooks.PeerCount = func(count int) {
		m.SetConnectedPeers("unknown", count)
	}
	p2pHost.Hooks.ReqRespTimeout = m.IncReqRespTimeout
	p2pHost.Hooks.ReqRespRequestSize = m.ObserveReqRespRequestSize
	p2pHost.Hooks.ReqRespResponseChunkSize = m.ObserveReqRespResponseChunkSize
}

// The prover owns a multi-GB resident arena; nodes that never prove (no
// validator keys, not aggregating) skip it and keep only the verifier. The
// proving FFI entry points lazily self-initialize, so an unexpected proving
// need still works — it just pays the init off the tick loop on first use.
func preinitializeXMSS(proving bool) error {
	if !proving {
		logger.Info(logger.Node, "no validator keys and not an aggregator; deferring XMSS prover init")
		return xmss.EnsureVerifierReady()
	}
	logger.Info(logger.Node, "pre-initializing XMSS prover (this takes ~45s)...")
	if err := xmss.EnsureProverReady(); err != nil {
		return err
	}
	logger.Info(logger.Node, "XMSS prover ready")
	return xmss.EnsureVerifierReady()
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
		// The block-request window slides with the responder's current slot, not its
		// head: a lagging head would advertise a lower floor and offer history the
		// spec lets us prune.
		func() uint64 {
			return types.CurrentSlot(s.Config().GenesisTime, uint64(time.Now().UnixMilli()))
		},
		func(startSlot, count uint64) ([]*types.SignedBlock, bool) {
			return s.GetCanonicalBlocksInRange(startSlot, count)
		},
	)
}

func startNodeNetworking(ctx context.Context, services *tasks.Group, n *node.Engine, s *store.ConsensusStore, p2pHost *p2p.Host, bootnodes []multiaddr.Multiaddr) {
	p2pHost.StartGossipListeners(n)
	services.Go(func() { n.Run(ctx) })

	syncDriver := syncer.NewSyncDriver(ctx, n, s, p2pHost)
	p2pHost.Hooks.PeerStatus = syncDriver.OnPeerConnected
	services.Go(syncDriver.Run)

	p2pHost.ConnectBootnodes(ctx, bootnodes)
	p2pHost.StartBootnodeRedial(ctx, bootnodes)
	p2pHost.ReannounceSubscriptionsAfter(5 * time.Second)
}
