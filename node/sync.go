package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/types"
)

// syncWithPeer exchanges status and fetches missing blocks from a single peer.
// It walks backwards from the peer's head to find blocks we're missing, then
// processes them in forward order.
func (n *Node) syncWithPeer(ctx context.Context, pid peer.ID) bool {
	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	if err != nil {
		n.log.Debug("status exchange failed", "peer", pid.String()[:16], "err", err)
		return false
	}
	n.log.Info("status exchanged",
		"peer", pid.String()[:16],
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_finalized_slot", peerStatus.Finalized.Slot,
	)

	if peerStatus.Head.Slot <= status.HeadSlot {
		return false
	}

	// Walk backwards: request blocks we don't have, collecting roots to fetch.
	var pending []*types.SignedBlockWithAttestation
	nextRoot := peerStatus.Head.Root
	const maxSyncDepth = 64

	for i := 0; i < maxSyncDepth; i++ {
		if _, ok := n.FC.GetBlock(nextRoot); ok {
			break // We have this block, chain is connected.
		}

		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, [][32]byte{nextRoot})
		if err != nil || len(blocks) == 0 {
			n.log.Debug("blocks_by_root failed during sync walk", "peer", pid.String()[:16], "err", err)
			break
		}

		sb := blocks[0]
		pending = append(pending, sb)
		nextRoot = sb.Message.Block.ParentRoot
	}

	// Process in forward order (oldest first).
	synced := 0
	for i := len(pending) - 1; i >= 0; i-- {
		sb := pending[i]
		if err := n.FC.ProcessBlock(sb); err != nil {
			n.log.Debug("sync block rejected", "slot", sb.Message.Block.Slot, "err", err)
		} else {
			n.log.Info("synced block", "slot", sb.Message.Block.Slot)
			synced++
		}
	}
	return synced > 0
}

// initialSync exchanges status with connected peers and requests any blocks
// we're missing. This allows a node that restarts mid-devnet to catch up.
func (n *Node) initialSync(ctx context.Context) {
	peers := n.Host.P2P.Network().Peers()
	for _, pid := range peers {
		n.syncWithPeer(ctx, pid)
	}
}
