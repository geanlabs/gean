package syncer

import "time"

const (
	blocksByRangeSyncThreshold = 2
	pollInterval               = 32 * time.Second
	peerStatusTimeout          = 10 * time.Second
)
