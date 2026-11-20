package p2p

import (
	"github.com/geanlabs/gean/internal/core/p2p/host"
	"github.com/geanlabs/gean/internal/core/p2p/peers"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/metrics"
)

// init points core p2p's measurement hooks at lean's metrics.
func init() {
	host.OnRequestOutcome = metrics.IncPeerRequestOutcome
	peers.OnScore = metrics.SetPeerScore
	peers.OnRemove = metrics.DeletePeerScore
	reqresp.OnTimeout = metrics.IncReqRespTimeout
	reqresp.OnRequest = metrics.ObserveReqRespRequestSize
	reqresp.OnResponseChunk = metrics.ObserveReqRespResponseChunkSize
}
