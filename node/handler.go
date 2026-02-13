package node

import (
	"fmt"

	"github.com/devylongs/gean/chain/forkchoice"
	"github.com/devylongs/gean/network/gossipsub"
	"github.com/devylongs/gean/network/reqresp"
	"github.com/devylongs/gean/observability/logging"
	"github.com/devylongs/gean/storage/memory"
	"github.com/devylongs/gean/types"
)

// registerHandlers wires up gossip subscriptions and req/resp protocol handlers.
func registerHandlers(n *Node, store *memory.Store, fc *forkchoice.Store) error {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)

	// Register req/resp handlers.
	reqresp.RegisterReqResp(n.Host.P2P, &reqresp.ReqRespHandler{
		OnStatus: func(req reqresp.Status) reqresp.Status {
			headSlot := uint64(0)
			if hb, ok := store.GetBlock(fc.Head); ok {
				headSlot = hb.Slot
			}
			return reqresp.Status{
				Finalized: fc.LatestFinalized,
				Head:      &types.Checkpoint{Root: fc.Head, Slot: headSlot},
			}
		},
		OnBlocksByRoot: func(roots [][32]byte) []*types.SignedBlock {
			var blocks []*types.SignedBlock
			for _, root := range roots {
				if b, ok := store.GetBlock(root); ok {
					blocks = append(blocks, &types.SignedBlock{Message: b, Signature: types.ZeroHash})
				}
			}
			return blocks
		},
	})

	// Subscribe to gossip.
	if err := gossipsub.SubscribeTopics(n.Host.Ctx, n.Topics, &gossipsub.GossipHandler{
		OnBlock: func(sb *types.SignedBlock) {
			blockRoot, _ := sb.Message.HashTreeRoot()
			gossipLog.Info("received block via gossip",
				"slot", sb.Message.Slot,
				"proposer", sb.Message.ProposerIndex,
				"block_root", logging.ShortHash(blockRoot),
			)
			if err := fc.ProcessBlock(sb.Message); err != nil {
				gossipLog.Warn("rejected gossip block",
					"slot", sb.Message.Slot,
					"err", err,
				)
			}
		},
		OnVote: func(sv *types.SignedVote) {
			fc.ProcessAttestation(sv)
		},
	}); err != nil {
		return fmt.Errorf("subscribe topics: %w", err)
	}

	return nil
}
