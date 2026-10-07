package p2p

import "github.com/libp2p/go-libp2p/core/peer"

type Hooks struct {
	GossipBlockSize       func(bytes int)
	GossipAttestationSize func(bytes int)
	GossipAggregationSize func(bytes int)
	PeerConnected         func(direction string)
	PeerDisconnected      func(direction, reason string)
	PeerCount             func(count int)
	PeerStatus            func(peerID peer.ID)
}
