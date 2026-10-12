package syncer

import (
	"context"
	"sync"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/lifecycle"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/store"
)

type SyncDriver struct {
	node  LocalNode
	store *store.ConsensusStore
	p2p   SyncDriverP2P
	ctx   context.Context

	mu       sync.Mutex
	inFlight map[libp2ppeer.ID]bool
	// horizonReported latches the beyond-window report so it is stated once per
	// episode rather than once per peer per poll.
	horizonReported bool

	// tasks tracks Run, the per-peer polls and new-peer status checks, which all
	// read the store, so shutdown can join them before closing it.
	tasks lifecycle.Group
}

func NewSyncDriver(ctx context.Context, node LocalNode, store *store.ConsensusStore, p2pHost SyncDriverP2P) *SyncDriver {
	if ctx == nil {
		ctx = context.Background()
	}
	return &SyncDriver{
		ctx:      ctx,
		node:     node,
		store:    store,
		p2p:      p2pHost,
		inFlight: make(map[libp2ppeer.ID]bool),
	}
}

// Start runs the driver on a tracked goroutine.
func (sd *SyncDriver) Start() {
	sd.tasks.Go(sd.Run)
}

// Wait blocks until Run and every poll have returned. Callers cancel the
// driver's context first.
func (sd *SyncDriver) Wait() {
	sd.tasks.Close()
}

func (sd *SyncDriver) Run() {
	if !sd.ready() {
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	logger.Info(logger.Sync, "sync driver started: poll_interval=%s threshold=%d slots",
		pollInterval, blocksByRangeSyncThreshold)

	for {
		select {
		case <-sd.ctx.Done():
			return
		case <-ticker.C:
			if sd.node.GetSyncStatus() == SyncSyncing {
				sd.refreshSyncFromPeers(sd.ctx)
			}
		}
	}
}
