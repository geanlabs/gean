package aggregation

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type Publisher interface {
	PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error
}

type Dispatch struct {
	Snapshot *Snapshot
	Slot     uint64
}

func RunWorker(
	ctx context.Context,
	dispatches <-chan Dispatch,
	consensusStore *store.ConsensusStore,
	cache *xmss.PubKeyCache,
	publisher Publisher,
	gate *proving.Gate,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch, ok := <-dispatches:
			if !ok {
				return
			}
			metrics.SetProvingQueueDepth("aggregation", len(dispatches))
			if dispatch.Snapshot == nil {
				continue
			}
			// The timeout bounds only how long we wait for the prover; once
			// proving has run, completed results are kept as long as the slot
			// is still current. Discarding finished, slot-valid aggregates
			// (and their signature deletes) starves the network of proofs and
			// regrows the next snapshot.
			acquireCtx, cancelAcquire := context.WithTimeout(ctx, 750*time.Millisecond)
			if gate != nil && !gate.Acquire(acquireCtx, false) {
				cancelAcquire()
				metrics.IncProofOperation("aggregation", "canceled")
				continue
			}
			cancelAcquire()

			workerStart := time.Now()
			aggs, payloads, deletes := aggregateFromSnapshot(dispatch.Snapshot, cache)
			if gate != nil {
				gate.Release(false)
			}
			if consensusStore.Time()/types.IntervalsPerSlot > dispatch.Slot {
				metrics.IncProofOperation("aggregation", "canceled")
				continue
			}
			applyAggregationMutations(consensusStore, payloads, deletes)
			publishCtx, cancelPublish := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
			publishAggregates(publishCtx, publisher, aggs)
			cancelPublish()
			metrics.IncProofOperation("aggregation", "success")
			metrics.ObserveProvingDuration("aggregation", time.Since(workerStart).Seconds())
			metrics.ObserveAggregationWorkerTotalTime(time.Since(workerStart).Seconds())
			logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v",
				dispatch.Slot, len(aggs), time.Since(workerStart))
		}
	}
}
