package sim

import (
	"context"
	"fmt"
	"strconv"

	"github.com/geanlabs/gean/net/syncer"
	"github.com/geanlabs/gean/types"
)

// network is one node's view of the simulated network. It implements
// node.Network: whatever its node publishes is delivered at once to every
// other node's engine through the same entry points real gossip uses, in node
// order, and fetches are answered from the other nodes' stores. Each recipient
// gets its own SSZ-decoded copy, as it would off the wire.
type network struct {
	cluster *Cluster
	self    int
}

func (n *network) peers(fn func(*Node)) {
	for i, peer := range n.cluster.nodes {
		if i != n.self {
			fn(peer)
		}
	}
}

func (n *network) PublishBlock(_ context.Context, block *types.SignedBlock) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return err
	}
	n.peers(func(peer *Node) {
		decoded := &types.SignedBlock{}
		if err := decoded.UnmarshalSSZ(data); err == nil {
			peer.Engine.OnBlock(decoded)
		}
	})
	return nil
}

func (n *network) PublishAttestation(_ context.Context, att *types.SignedAttestation, _ uint64) error {
	data, err := att.MarshalSSZ()
	if err != nil {
		return err
	}
	n.peers(func(peer *Node) {
		decoded := &types.SignedAttestation{}
		if err := decoded.UnmarshalSSZ(data); err == nil {
			peer.Engine.OnGossipAttestation(decoded)
		}
	})
	return nil
}

func (n *network) PublishAggregatedAttestation(_ context.Context, agg *types.SignedAggregatedAttestation) error {
	data, err := agg.MarshalSSZ()
	if err != nil {
		return err
	}
	n.peers(func(peer *Node) {
		decoded := &types.SignedAggregatedAttestation{}
		if err := decoded.UnmarshalSSZ(data); err == nil {
			peer.Engine.OnGossipAggregatedAttestation(decoded)
		}
	})
	return nil
}

func (n *network) FetchBlocksByRootBatchWithRetry(_ context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	var found []*types.SignedBlock
	var missing [][32]byte
	for _, root := range roots {
		var block *types.SignedBlock
		n.peers(func(peer *Node) {
			if block == nil {
				block = peer.Store.GetSignedBlock(root)
			}
		})
		if block == nil {
			missing = append(missing, root)
			continue
		}
		found = append(found, block)
	}
	return found, missing, nil
}

func (n *network) ConnectedPeers() int { return len(n.cluster.nodes) - 1 }

func (n *network) MeshPeerCount() int { return len(n.cluster.nodes) - 1 }

func (n *network) TopicMeshSizes() map[string]int { return nil }

// Peers, SendStatusRequest and FetchBlocksByRange make the network a
// syncer.SyncDriverP2P too, so real-time clusters run the real sync driver.
// A peer's ID is its node index.
func (n *network) Peers() []syncer.PeerID {
	var ids []syncer.PeerID
	for i := range n.cluster.nodes {
		if i != n.self {
			ids = append(ids, syncer.PeerID(strconv.Itoa(i)))
		}
	}
	return ids
}

func (n *network) SendStatusRequest(_ context.Context, id syncer.PeerID, _ *types.Status) (*types.Status, error) {
	peer, err := n.peer(id)
	if err != nil {
		return nil, err
	}
	return peer.Store.Status(), nil
}

func (n *network) FetchBlocksByRange(_ context.Context, id syncer.PeerID, startSlot, count uint64) ([]*types.SignedBlock, error) {
	peer, err := n.peer(id)
	if err != nil {
		return nil, err
	}
	blocks, _ := peer.Store.GetCanonicalBlocksInRange(startSlot, count)
	return blocks, nil
}

func (n *network) peer(id syncer.PeerID) (*Node, error) {
	i, err := strconv.Atoi(string(id))
	if err != nil || i < 0 || i >= len(n.cluster.nodes) || i == n.self {
		return nil, fmt.Errorf("unknown peer %q", id)
	}
	return n.cluster.nodes[i], nil
}
