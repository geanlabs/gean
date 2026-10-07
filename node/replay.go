package node

import "github.com/geanlabs/gean/logger"

func (e *Engine) replayPendingAttestations(headRoot [32]byte) {
	pending := e.PendingAttestations.Drain(headRoot)
	if len(pending) == 0 {
		return
	}
	logger.Info(logger.Gossip, "replaying %d buffered attestations for newly arrived head=0x%x",
		len(pending), headRoot)
	// Replayed attestations re-enter through the gossip path, so they are
	// verified by the same worker and dropped under the same overload.
	for _, att := range pending {
		e.OnGossipAttestation(att)
	}
}
