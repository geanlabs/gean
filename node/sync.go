package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
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

	// If peer is strictly behind us, or at the same slot with the same head root,
	// there is nothing to sync from this peer.
	if peerStatus.Head.Slot < status.HeadSlot ||
		(peerStatus.Head.Slot == status.HeadSlot && peerStatus.Head.Root == status.Head) {
		return false
	}

	// Walk backwards: request blocks we don't have, collecting roots to fetch.
	var pending []*types.SignedBlockWithAttestation
	nextRoot := peerStatus.Head.Root
	// Late-join nodes can be hundreds of slots behind. Use a backlog-sized walk
	// rather than a fixed depth, otherwise we only fetch a disconnected suffix.
	backlog := uint64(1)
	if peerStatus.Head.Slot > status.HeadSlot {
		backlog = peerStatus.Head.Slot - status.HeadSlot
	}
	maxSyncDepth := int(backlog + 16)
	const maxSyncDepthCap = 32768
	if maxSyncDepth > maxSyncDepthCap {
		maxSyncDepth = maxSyncDepthCap
	}

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

	// If we could not reach any known ancestor, imported blocks would remain
	// disconnected and fail with "parent state not found".
	if _, ok := n.FC.GetBlock(nextRoot); !ok {
		n.log.Debug("sync walk did not reach known ancestor",
			"peer", pid.String()[:16],
			"fetched", len(pending),
			"max_depth", maxSyncDepth,
		)
		return false
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

// isBehindPeerFinalization reports whether local head/finalized is behind the
// highest finalized slot advertised by connected peers.
func (n *Node) isBehindPeerFinalization(ctx context.Context, status forkchoice.ChainStatus) (bool, uint64) {
	maxPeerFinalizedSlot := status.FinalizedSlot
	peers := n.Host.P2P.Network().Peers()
	if len(peers) == 0 {
		return false, maxPeerFinalizedSlot
	}

	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	for _, pid := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		peerStatus, err := reqresp.RequestStatus(peerCtx, n.Host.P2P, pid, ourStatus)
		cancel()
		if err != nil || peerStatus.Finalized == nil {
			continue
		}
		if peerStatus.Finalized.Slot > maxPeerFinalizedSlot {
			maxPeerFinalizedSlot = peerStatus.Finalized.Slot
		}
	}

	// Gate duties only when our head is behind peer finalization, meaning we
	// are missing finalized chain blocks. Do not gate solely on finalized-slot
	// lag, which can transiently trail while head is already caught up.
	behind := status.HeadSlot < maxPeerFinalizedSlot
	return behind, maxPeerFinalizedSlot
}
