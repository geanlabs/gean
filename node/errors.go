package node

import "errors"

// Node errors. Callers may use errors.Is to check for them.
var (
	ErrSyncInProgress = errors.New("sync in progress")
)
