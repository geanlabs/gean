package syncer

import "github.com/geanlabs/gean/internal/lean/p2p"

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

func (sd *SyncDriver) makeStatusMessage() *p2p.StatusMessage {
	finalized := sd.store.LatestFinalized()
	return &p2p.StatusMessage{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.store.Head(),
		HeadSlot:      sd.store.HeadSlot(),
	}
}
