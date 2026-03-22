package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/types"
)

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

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
		n.log.Debug("status exchange failed", "peer_id", pid.String(), "err", err)
		return false
	}
	n.log.Info("status exchanged",
		"peer_id", pid.String(),
		"local_head_slot", status.HeadSlot,
		"local_head_root", logging.LongHash(status.Head),
		"local_finalized_slot", status.FinalizedSlot,
		"local_finalized_root", logging.LongHash(status.FinalizedRoot),
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_head_root", logging.LongHash(peerStatus.Head.Root),
		"peer_finalized_slot", peerStatus.Finalized.Slot,
		"peer_finalized_root", logging.LongHash(peerStatus.Finalized.Root),
	)

	// Skip sync only if peer is strictly behind us, or at the exact same position.
	// If peer is at the same slot but with a different head root, we should still
	// sync to ensure we have their chain (potential re-org or fork).
	if peerStatus.Head.Slot < status.HeadSlot {
		return false
	}
	if peerStatus.Head.Slot == status.HeadSlot && peerStatus.Head.Root == status.Head {
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
		// Check for state existence, not just block. ProcessBlock requires the
		// parent state to succeed, so we need to walk back until we find a root
		// for which we have the state.
		if n.FC.HasState(nextRoot) {
			break // We have state for this block, chain is connected.
		}

		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, [][32]byte{nextRoot})
		if err != nil || len(blocks) == 0 {
			n.log.Debug("blocks_by_root failed during sync walk",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
				"err", err,
			)
			break
		}

		sb := blocks[0]
		pending = append(pending, sb)
		nextRoot = sb.Message.Block.ParentRoot
	}

	// If we could not reach any known ancestor with state, imported blocks would
	// fail with "parent state not found".
	if !n.FC.HasState(nextRoot) {
		n.log.Debug("sync walk did not reach known ancestor with state",
			"peer_id", pid.String(),
			"ancestor_root", logging.LongHash(nextRoot),
			"fetched", len(pending),
			"max_depth", maxSyncDepth,
		)
		return false
	}

	// Process in forward order (oldest first).
	synced := 0
	total := len(pending)
	for i := len(pending) - 1; i >= 0; i-- {
		sb := pending[i]
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if err := n.FC.ProcessBlock(sb); err != nil {
			n.log.Debug("sync block rejected",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"err", err,
			)
		} else {
			synced++
			n.log.Info("synced block",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"peer_id", pid.String(),
				"progress", fmt.Sprintf("%d/%d", synced, total),
			)
		}
	}
	return synced > 0
}

// recoverMissingParentSync attempts to fill a missing parent chain by syncing with
// connected peers, then checks whether the requested parent state became available.
func (n *Node) recoverMissingParentSync(ctx context.Context, parentRoot [32]byte) bool {
	if n.FC.HasState(parentRoot) {
		return true
	}

	for _, pid := range n.Host.P2P.Network().Peers() {
		n.syncWithPeer(ctx, pid)
		if n.FC.HasState(parentRoot) {
			return true
		}
	}
	return false
}

// initialSync exchanges status with connected peers and requests any blocks
// we're missing. This allows a node that restarts mid-devnet to catch up.
func (n *Node) initialSync(ctx context.Context) {
	peers := n.Host.P2P.Network().Peers()
	n.log.Info("initial sync starting", "peer_count", len(peers))
	for _, pid := range peers {
		n.syncWithPeer(ctx, pid)
	}
	status := n.FC.GetStatus()
	n.log.Info("initial sync completed",
		"head_slot", status.HeadSlot,
		"head_root", logging.LongHash(status.Head),
		"justified_slot", status.JustifiedSlot,
		"justified_root", logging.LongHash(status.JustifiedRoot),
		"finalized_slot", status.FinalizedSlot,
		"finalized_root", logging.LongHash(status.FinalizedRoot),
	)
}

// isBehindPeers reports whether our head is behind the highest head slot
// advertised by connected peers. This fires even when finalization is stalled,
// ensuring we sync from peers who have blocks we haven't yet received via gossip.
func (n *Node) isBehindPeers(ctx context.Context, status forkchoice.ChainStatus) (bool, uint64) {
	maxPeerHeadSlot := status.HeadSlot
	peers := n.Host.P2P.Network().Peers()
	if len(peers) == 0 {
		return false, maxPeerHeadSlot
	}

	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	for _, pid := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		peerStatus, err := reqresp.RequestStatus(peerCtx, n.Host.P2P, pid, ourStatus)
		cancel()
		if err != nil || peerStatus.Head == nil {
			continue
		}
		if peerStatus.Head.Slot > maxPeerHeadSlot {
			maxPeerHeadSlot = peerStatus.Head.Slot
		}
	}

	behind := status.HeadSlot < maxPeerHeadSlot
	return behind, maxPeerHeadSlot
}
