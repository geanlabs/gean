package node

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/types"
)

// syncMu prevents concurrent sync attempts to the same peer.
var syncMu sync.Mutex

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

	return n.fetchMissingBlocks(ctx, pid, peerStatus)
}

// fetchMissingBlocks is called when we know the peer has a higher head than us.
// It walks backwards from the peer's head root until it finds a block we have,
// then replays the chain forward. Exported so it can be called from the
// connection-notification path as well.
func (n *Node) fetchMissingBlocks(ctx context.Context, pid peer.ID, peerStatus *reqresp.Status) bool {
	// Serialise sync attempts so we don't make duplicate requests.
	syncMu.Lock()
	defer syncMu.Unlock()

	// Re-check head inside the lock; another goroutine may have already synced.
	status := n.FC.GetStatus()
	if peerStatus.Head.Slot <= status.HeadSlot {
		return false
	}

	// Walk backwards: request blocks we don't have, collecting roots to fetch.
	var pending []*types.SignedBlockWithAttestation
	nextRoot := peerStatus.Head.Root
	const maxSyncDepth = 64

	for i := 0; i < maxSyncDepth; i++ {
		if _, ok := n.FC.GetBlock(nextRoot); ok {
			break // We have this block; chain is connected.
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

// initialSync exchanges status with all currently connected peers and requests
// any blocks we're missing. Called once on startup (after bootnodes connect)
// to catch up when restarting mid-devnet.
func (n *Node) initialSync(ctx context.Context) {
	peers := n.Host.P2P.Network().Peers()
	for _, pid := range peers {
		n.syncWithPeer(ctx, pid)
	}
}

// onPeerConnected is called whenever a new peer connects. If they are ahead of
// us we immediately fetch their chain. This is the key path that makes node0
// catch up after node1/node2 reconnect to it via the retry loop.
func (n *Node) onPeerConnected(ctx context.Context, pid peer.ID) {
	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	if err != nil {
		n.log.Debug("status exchange on connect failed", "peer", pid.String()[:16], "err", err)
		return
	}
	n.log.Info("status exchanged",
		"peer", pid.String()[:16],
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_finalized_slot", peerStatus.Finalized.Slot,
	)

	if peerStatus.Head.Slot > status.HeadSlot {
		n.log.Info("peer is ahead, syncing", "peer", pid.String()[:16],
			"peer_slot", peerStatus.Head.Slot, "our_slot", status.HeadSlot)
		n.fetchMissingBlocks(ctx, pid, peerStatus)
	}
}