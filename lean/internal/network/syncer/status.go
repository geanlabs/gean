package syncer

import "github.com/geanlabs/gean/lean/internal/network"

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

func (sd *SyncDriver) makeStatusMessage() *network.StatusMessage {
	if sd == nil || sd.store == nil {
		return nil
	}

	finalized := sd.store.LatestFinalized()
	return &network.StatusMessage{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.store.Head(),
		HeadSlot:      sd.store.HeadSlot(),
	}
}
