package node

// SyncStatus is the typed view of the node's sync state, used by SyncDriver
// to decide whether to poll peers for backfill. The String() values are kept
// stable as they're emitted to the lean_node_sync_status Prometheus metric
// and consumed by operator dashboards.
type SyncStatus int

const (
	// SyncIdle indicates no peers are connected — no progress possible.
	SyncIdle SyncStatus = iota
	// SyncSyncing indicates peers are connected but the head is more than
	// SyncLagSlots behind wall-clock — backfill polling is appropriate.
	SyncSyncing
	// SyncSynced indicates the head is within SyncLagSlots of wall-clock —
	// no polling required.
	SyncSynced
)

// String returns the metric label used by lean_node_sync_status. Stable.
func (s SyncStatus) String() string {
	switch s {
	case SyncIdle:
		return "idle"
	case SyncSyncing:
		return "syncing"
	case SyncSynced:
		return "synced"
	}
	return "unknown"
}
