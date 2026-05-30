package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var metricNodeSyncStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "lean_node_sync_status", Help: "Node sync status (one of idle/syncing/synced is set to 1)",
}, []string{"status"})

// SetSyncStatus sets the active sync status to 1 and the others to 0.
// Valid values: "idle", "syncing", "synced".
func SetSyncStatus(status string) {
	for _, s := range []string{"idle", "syncing", "synced"} {
		if s == status {
			metricNodeSyncStatus.WithLabelValues(s).Set(1)
		} else {
			metricNodeSyncStatus.WithLabelValues(s).Set(0)
		}
	}
}
