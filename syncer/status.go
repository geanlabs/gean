package syncer

import "github.com/geanlabs/gean/p2p"

func (sd *SyncDriver) makeStatusMessage() *p2p.StatusMessage {
	if sd == nil || sd.store == nil {
		return nil
	}

	finalized := sd.store.LatestFinalized()
	return &p2p.StatusMessage{
		FinalizedRoot: finalized.Root,
		FinalizedSlot: finalized.Slot,
		HeadRoot:      sd.store.Head(),
		HeadSlot:      sd.store.HeadSlot(),
	}
}
