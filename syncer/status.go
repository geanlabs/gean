package syncer

import "github.com/geanlabs/gean/types"

func (sd *SyncDriver) makeStatusMessage() *types.Status {
	if sd == nil || sd.store == nil {
		return nil
	}

	return sd.store.Status()
}
