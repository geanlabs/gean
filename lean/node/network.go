package node

import (
	"context"
	"time"

	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/internal/lifecycle"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/crypto/xmss"
	"github.com/geanlabs/gean/lean/internal/chain"
	"github.com/geanlabs/gean/lean/internal/network"
	"github.com/geanlabs/gean/lean/internal/network/syncer"
	"github.com/geanlabs/gean/lean/internal/store"
)

func setupP2P(ctx context.Context, cfg Config, keyManager *xmss.KeyManager) (*network.Host, error) {
	var validatorIDs []uint64
	if keyManager != nil {
		validatorIDs = keyManager.ValidatorIDs()
	}

	p2pHost, err := network.NewHost(ctx, cfg.NodeKey, cfg.GossipPort, cfg.CommitteeCount, validatorIDs, cfg.IsAggregator, cfg.AggregateSubnetIDs)
	if err != nil {
		return nil, err
	}

	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", p2pHost.PeerID(), cfg.GossipPort)
	return p2pHost, nil
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

func registerReqRespHandlers(p2pHost *network.Host, s *store.ConsensusStore) {
	p2pHost.RegisterReqRespHandlers(
		func() *network.StatusMessage {
			finalized := s.LatestFinalized()
			return &network.StatusMessage{
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

// startNodeNetworking starts the engine and the sync driver and connects to the
// network. The re-announce timer runs on tasks.
func startNodeNetworking(ctx context.Context, n *chain.Engine, s *store.ConsensusStore, p2pHost *network.Host, bootnodes []multiaddr.Multiaddr, tasks *lifecycle.Group) *syncer.SyncDriver {
	p2pHost.StartGossipListeners(n)
	n.Start(ctx)

	syncDriver := syncer.NewSyncDriver(ctx, n, s, p2pHost)
	p2pHost.Hooks.PeerStatus = syncDriver.OnPeerConnected
	syncDriver.Start()

	p2pHost.ConnectBootnodes(ctx, bootnodes)
	p2pHost.StartBootnodeRedial(ctx, bootnodes)
	scheduleSubscriptionReannounce(ctx, p2pHost, tasks)
	return syncDriver
}

func scheduleSubscriptionReannounce(ctx context.Context, p2pHost *network.Host, tasks *lifecycle.Group) {
	tasks.Go(func() {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
		if err := p2pHost.ReannounceSubscriptions(); err != nil {
			logger.Error(logger.Network, "re-announce subscriptions failed: %v", err)
		}
	})
}
