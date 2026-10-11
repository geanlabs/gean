package node

import "github.com/geanlabs/gean/internal/logger"

// replayPendingAttestations re-submits the attestations buffered for headRoot
// through the gossip queue. One root holds at most PendingAttestationsPerRootCap,
// but importing a chain of buffered blocks replays each in turn, up to
// PendingAttestationsTotalCap, which can outrun the queue during catch-up. The
// excess is dropped like any gossip overflow; those votes name heads we lacked,
// so they are old.
func (e *Engine) replayPendingAttestations(headRoot [32]byte) {
	pending := e.PendingAttestations.Drain(headRoot)
	if len(pending) == 0 {
		return
	}
	logger.Info(logger.Gossip, "replaying %d buffered attestations for newly arrived head=0x%x",
		len(pending), headRoot)
	for _, att := range pending {
		e.OnGossipAttestation(att)
	}
}
