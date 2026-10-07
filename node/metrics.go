package node

import (
	"time"

	"github.com/geanlabs/gean/metrics"
)

func (e *Engine) initMetrics() {
	metrics.SetSyncStatus("idle")
	metrics.SetNodeStartTime(float64(time.Now().Unix()))
	metrics.SetAttestationCommitteeCount(e.committeeCount)

	if e.keys == nil {
		return
	}
	vids := e.keys.ValidatorIDs()
	metrics.SetValidatorsCount(len(vids))
	if len(vids) > 0 && e.committeeCount > 0 {
		metrics.SetAttestationCommitteeSubnet(vids[0] % e.committeeCount)
	}
}
