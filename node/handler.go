package node

import (
	"fmt"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

// registerHandlers wires up gossip subscriptions and req/resp protocol handlers.
func registerHandlers(n *Node, fc *forkchoice.Store) error {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)

	// Register req/resp handlers.
	reqresp.RegisterReqResp(n.Host.P2P, &reqresp.ReqRespHandler{
		OnStatus: func(req reqresp.Status) reqresp.Status {
			status := fc.GetStatus()
			return reqresp.Status{
				Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
				Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
			}
		},
		OnBlocksByRoot: func(roots [][32]byte) []*types.SignedBlockWithAttestation {
			var blocks []*types.SignedBlockWithAttestation
			for _, root := range roots {
				if sb, ok := fc.GetSignedBlock(root); ok {
					blocks = append(blocks, sb)
				}
			}
			return blocks
		},
	})

	// Subscribe to gossip.
	if err := gossipsub.SubscribeTopics(n.Host.Ctx, n.Topics, &gossipsub.GossipHandler{
		OnBlock: func(sb *types.SignedBlockWithAttestation) {
			block := sb.Message.Block
			blockRoot, _ := block.HashTreeRoot()
			gossipLog.Info("received block via gossip",
				"slot", block.Slot,
				"proposer", block.ProposerIndex,
				"block_root", logging.ShortHash(blockRoot),
			)
			if err := fc.ProcessBlock(sb); err != nil {
				gossipLog.Warn("rejected gossip block",
					"slot", block.Slot,
					"err", err,
				)
			}
		},
		OnAttestation: func(sa *types.SignedAttestation) {
			fc.ProcessAttestation(sa)
		},
		OnAggregatedAttestation: func(agg *types.AggregatedAttestation) {
			gossipLog.Debug("received aggregated attestation via gossip",
				"slot", agg.Data.Slot,
				"num_sigs", len(agg.AggregatedSignature)/types.XMSSSignatureSize,
			)
			fc.ProcessAggregatedAttestation(agg)
		},
	}); err != nil {
		return fmt.Errorf("subscribe topics: %w", err)
	}

	return nil
}
