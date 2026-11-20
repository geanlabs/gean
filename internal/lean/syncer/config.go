package syncer

import "time"

const (
	blocksByRangeSyncThreshold = 2
	pollInterval               = 32 * time.Second

	// forkReconcileFinalizedThreshold is how far our finalized checkpoint may
	// fall behind a peer's before we treat it as a fork we are marooned on and
	// range-backfill to reconcile. Cross-node finalized skew is normally 0-2
	// slots on a shared chain, so this stays clear of healthy operation.
	forkReconcileFinalizedThreshold = 8

	// maxEmptyRangeBatches ends a pass after this many consecutive empty answers
	// from the status peer. Peers serve only MinSlotsForBlockRequests of history,
	// which this many MaxRequestBlocks batches cover, so an honest peer holding
	// blocks further on cannot answer empty more often than this.
	maxEmptyRangeBatches = 4
)
