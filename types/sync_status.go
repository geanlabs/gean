package types

// SyncStatus is how far a node's head is from the network's current slot.
type SyncStatus int

const (
	SyncIdle SyncStatus = iota
	SyncSyncing
	SyncSynced
)

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
