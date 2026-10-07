package syncer

import "github.com/geanlabs/gean/types"

func (sd *SyncDriver) makeStatusMessage() *types.Status {
	if sd == nil || sd.store == nil {
		return nil
	}

	finalized := sd.store.LatestFinalized()
	return &types.Status{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.store.Head(),
		HeadSlot:      sd.store.HeadSlot(),
	}
}
