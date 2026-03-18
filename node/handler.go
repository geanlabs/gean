package node

import (
	"fmt"
	"log/slog"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

// registerReqRespHandlers wires up request/response protocol handlers.
// This is called during node initialization so sync can work.
func registerReqRespHandlers(n *Node, fc *forkchoice.Store) {
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
}

// registerGossipHandlers subscribes to gossip topics for blocks and attestations.
// This is called AFTER initial sync to prevent processing gossip blocks before
// the chain is connected to the network's canonical chain.
func (n *Node) registerGossipHandlers() error {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)

	// Subscribe to gossip.
	if err := gossipsub.SubscribeTopics(n.Host.Ctx, n.Topics, &gossipsub.GossipHandler{
		OnBlock: func(sb *types.SignedBlockWithAttestation) {
			block := sb.Message.Block
			blockRoot, _ := block.HashTreeRoot()
			gossipLog.Info("received block via gossip",
				"slot", block.Slot,
				"proposer", block.ProposerIndex,
				"block_root", logging.LongHash(blockRoot),
			)
			if err := n.FC.ProcessBlock(sb); err != nil {
				if isMissingParentStateErr(err) {
					gossipLog.Warn("parent state missing for gossip block, attempting recovery",
						"slot", block.Slot,
						"block_root", logging.LongHash(blockRoot),
						"parent_root", logging.LongHash(block.ParentRoot),
					)
					if n.recoverMissingParentSync(n.Host.Ctx, block.ParentRoot) {
						if retryErr := n.FC.ProcessBlock(sb); retryErr == nil {
							gossipLog.Info("accepted gossip block after parent recovery",
								"slot", block.Slot,
								"block_root", logging.LongHash(blockRoot),
							)
							// Process any pending children now that this block is available.
							n.processPendingChildren(blockRoot, gossipLog)
							return
						} else {
							err = retryErr
						}
					}
					// Cache the block for later processing when parent becomes available.
					n.PendingBlocks.Add(sb)
					gossipLog.Info("cached pending block awaiting parent",
						"slot", block.Slot,
						"block_root", logging.LongHash(blockRoot),
						"parent_root", logging.LongHash(block.ParentRoot),
						"pending_count", n.PendingBlocks.Len(),
					)
					return
				}
				gossipLog.Warn("rejected gossip block",
					"slot", block.Slot,
					"err", err,
				)
				return
			}
			// Block processed successfully - check for pending children.
			n.processPendingChildren(blockRoot, gossipLog)
		},
		OnAttestation: func(sa *types.SignedAttestation) {
			n.FC.ProcessSubnetAttestation(sa)
		},
		OnAggregatedAttestation: func(saa *types.SignedAggregatedAttestation) {
			gossipLog.Debug("received aggregated attestation via gossip",
				"slot", saa.Data.Slot,
			)
			n.FC.ProcessAggregatedAttestation(saa)
		},
	}); err != nil {
		return fmt.Errorf("subscribe topics: %w", err)
	}

	return nil
}

// processPendingChildren processes any cached blocks that were waiting for this parent.
// This implements the leanSpec requirement to process cached blocks when their parent arrives.
func (n *Node) processPendingChildren(parentRoot [32]byte, log *slog.Logger) {
	children := n.PendingBlocks.GetChildrenOf(parentRoot)
	for _, sb := range children {
		block := sb.Message.Block
		blockRoot, _ := block.HashTreeRoot()

		if err := n.FC.ProcessBlock(sb); err != nil {
			// Still can't process - may be missing a deeper ancestor.
			log.Debug("pending child still not processable",
				"slot", block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"err", err,
			)
			continue
		}

		// Successfully processed - remove from pending and recurse.
		n.PendingBlocks.Remove(blockRoot)
		log.Info("processed pending child block",
			"slot", block.Slot,
			"block_root", logging.LongHash(blockRoot),
			"parent_root", logging.LongHash(parentRoot),
		)

		// Recursively process any children of this block.
		n.processPendingChildren(blockRoot, log)
	}
}
