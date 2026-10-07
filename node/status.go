package node

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/types"
)

const SyncLagSlots = 2

// gossipMeshSampleInterval paces the mesh-peer gauge. It is an observability
// sample, not a control input, so it runs well below the slot cadence.
const gossipMeshSampleInterval = 10 * time.Second

// tickAgeSampleInterval paces the stall detector. A slot is 4s, so a second is
// fine-grained enough to distinguish a slow tick from a stopped loop.
const tickAgeSampleInterval = time.Second

// storageSizeSampleInterval paces the per-table storage-size gauge. Table sizes
// move on compaction and pruning, not per block, so a minute is ample.
const storageSizeSampleInterval = time.Minute

func (e *Engine) updateSyncStatus(currentSlot uint64) {
	status := e.computeSyncStatus(currentSlot)
	metrics.SetSyncStatus(status.String())
}

func (e *Engine) computeSyncStatus(currentSlot uint64) types.SyncStatus {
	if e.Network != nil && e.Network.ConnectedPeers() == 0 {
		return types.SyncIdle
	}
	headSlot := e.Store.HeadSlot()
	if currentSlot <= headSlot || currentSlot-headSlot <= SyncLagSlots {
		return types.SyncSynced
	}
	return types.SyncSyncing
}

func (e *Engine) GetSyncStatus() types.SyncStatus {
	return e.computeSyncStatus(e.currentSlot(e.nowMs()))
}

func (e *Engine) logChainStatus(currentSlot uint64) {
	metrics.SampleProcessRSS()
	headRoot := e.Store.Head()
	headHeader := e.Store.GetBlockHeader(headRoot)
	justified := e.Store.LatestJustified()
	finalized := e.Store.LatestFinalized()

	headSlot := uint64(0)
	parentRoot := types.ZeroRoot
	stateRoot := types.ZeroRoot
	if headHeader != nil {
		headSlot = headHeader.Slot
		parentRoot = headHeader.ParentRoot
		stateRoot = headHeader.StateRoot
	}

	behind := uint64(0)
	if currentSlot > headSlot {
		behind = currentSlot - headSlot
	}

	peerCount := 0
	if e.Network != nil {
		peerCount = e.Network.ConnectedPeers()
	}

	gossipSigs := e.Store.AttestationSignatures().Len()
	knownPayloads := e.Store.KnownPayloads().Len()
	fcNodesCount := 0
	if e.FC != nil {
		fcNodesCount = e.FC.Len()
	}

	// Read the sample runGossipMeshGauge cached rather than querying pubsub here:
	// the query blocks on the pubsub event loop, and this runs on the tick loop.
	meshInfo := ""
	if sizes := e.topicMeshSizes.Load(); sizes != nil {
		for topic, size := range *sizes {
			meshInfo += fmt.Sprintf("\n  %-60s mesh_peers=%d", topic, size)
		}
	}

	logger.Info(logger.Chain, "\n\n+===============================================================+\n  CHAIN STATUS: Current Slot: %d | Head Slot: %d | Behind: %d\n+---------------------------------------------------------------+\n  Connected Peers:    %d\n+---------------------------------------------------------------+\n  Head Block Root:    0x%x\n  Parent Block Root:  0x%x\n  State Root:         0x%x\n+---------------------------------------------------------------+\n  Latest Justified:   Slot %6d | Root: 0x%x\n  Latest Finalized:   Slot %6d | Root: 0x%x\n+---------------------------------------------------------------+\n  Gossip Sigs: %d | Known Payloads: %d | FC Nodes: %d\n+---------------------------------------------------------------+\n  Topics:%s\n+===============================================================+\n",
		currentSlot, headSlot, behind,
		peerCount,
		headRoot, parentRoot, stateRoot,
		justified.Slot, justified.Root,
		finalized.Slot, finalized.Root,
		gossipSigs, knownPayloads, fcNodesCount,
		meshInfo)
}

// runGossipMeshGauge samples the mesh-peer gauge on its own goroutine. The
// underlying ListPeers is a synchronous round-trip through the libp2p pubsub
// event loop, so on a node busy serving req/resp it can block for seconds. Off
// the tick loop that is a slow gauge; on it, it delayed store.OnTick and stalled
// the store clock. Sampling is coarse by nature — a slow cadence loses nothing.
func (e *Engine) runGossipMeshGauge(ctx context.Context) {
	if e.Network == nil {
		return
	}
	ticker := time.NewTicker(gossipMeshSampleInterval)
	defer ticker.Stop()
	e.sampleGossipMesh()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sampleGossipMesh()
		}
	}
}

// runTickAgeGauge publishes how long it has been since the dispatch loop last
// began a tick. It must live off that loop: the existing tick-interval histogram
// is observed inside onTick, so a fully blocked loop produces no observations at
// all and the metric goes silent exactly when it should be alarming.
func (e *Engine) runTickAgeGauge(ctx context.Context) {
	ticker := time.NewTicker(tickAgeSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := e.lastTickMs.Load()
			if last == 0 {
				continue
			}
			age := time.Since(time.UnixMilli(last)).Seconds()
			if age < 0 {
				age = 0
			}
			metrics.SetTickAge(age)
		}
	}
}

// runStorageSizeGauge samples the per-table storage-size gauge off the tick
// loop. Even with a metadata-based estimate this is not work the dispatch
// goroutine should carry, and a size gauge loses nothing to a slow cadence.
func (e *Engine) runStorageSizeGauge(ctx context.Context) {
	ticker := time.NewTicker(storageSizeSampleInterval)
	defer ticker.Stop()
	e.recordTableBytes(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.recordTableBytes(ctx)
		}
	}
}

func (e *Engine) sampleGossipMesh() {
	metrics.SetGossipMeshPeers(e.Network.MeshPeerCount())
	sizes := e.Network.TopicMeshSizes()
	e.topicMeshSizes.Store(&sizes)
}
