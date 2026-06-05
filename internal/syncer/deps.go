package syncer

import "context"

func (sd *SyncDriver) ready() bool {
	return sd != nil && sd.node != nil && sd.store != nil && sd.p2p != nil
}

func (sd *SyncDriver) contextOrDefault(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	if sd != nil && sd.ctx != nil {
		return sd.ctx
	}
	return context.Background()
}
