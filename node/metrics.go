package node

import (
	"time"
)

func (e *Engine) initMetrics() {
	e.metrics.SetSyncStatus("idle")
	e.metrics.SetNodeStartTime(float64(time.Now().Unix()))
	e.metrics.SetAttestationCommitteeCount(e.committeeCount)

	if e.keys == nil {
		return
	}
	vids := e.keys.ValidatorIDs()
	e.metrics.SetValidatorsCount(len(vids))
	if len(vids) > 0 && e.committeeCount > 0 {
		e.metrics.SetAttestationCommitteeSubnet(vids[0] % e.committeeCount)
	}
}
