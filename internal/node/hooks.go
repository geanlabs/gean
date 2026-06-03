package node

import "github.com/geanlabs/gean/internal/metrics"

func (e *Engine) configureP2PHooks() {
	if e.P2P == nil {
		return
	}
	e.P2P.Hooks.GossipBlockSize = metrics.ObserveGossipBlockSize
	e.P2P.Hooks.GossipAttestationSize = metrics.ObserveGossipAttestationSize
	e.P2P.Hooks.GossipAggregationSize = metrics.ObserveGossipAggregationSize
	e.P2P.Hooks.PeerConnected = func(direction string) {
		metrics.IncPeerConnection(direction, "success")
	}
	e.P2P.Hooks.PeerDisconnected = func(direction, reason string) {
		metrics.IncPeerDisconnection(direction, reason)
	}
	e.P2P.Hooks.PeerCount = func(count int) {
		metrics.SetConnectedPeers("unknown", count)
	}
}
