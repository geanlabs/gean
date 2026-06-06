package node

import "github.com/geanlabs/gean/internal/logger"

func (e *Engine) replayPendingAttestations(headRoot [32]byte) {
	pending := e.PendingAttestations.Drain(headRoot)
	if len(pending) == 0 {
		return
	}
	logger.Info(logger.Gossip, "replaying %d buffered attestations for newly arrived head=0x%x",
		len(pending), headRoot)
	for _, att := range pending {
		att := att
		go e.onGossipAttestation(att)
	}
}
