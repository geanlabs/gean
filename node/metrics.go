package node

import (
	"time"

	"github.com/geanlabs/gean/metrics"
)

func (e *Engine) initMetrics() {
	metrics.SetSyncStatus("idle")
	metrics.SetNodeStartTime(float64(time.Now().Unix()))
	metrics.SetAttestationCommitteeCount(e.CommitteeCount)

	if e.Keys == nil {
		return
	}
	vids := e.Keys.ValidatorIDs()
	metrics.SetValidatorsCount(len(vids))
	if len(vids) > 0 && e.CommitteeCount > 0 {
		metrics.SetAttestationCommitteeSubnet(vids[0] % e.CommitteeCount)
	}
}
