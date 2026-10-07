package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/logger"
)

// timeEvent records how long one dispatch-loop event took. Every case runs on
// the single goroutine that also keeps the slot clock, so an unattributed slow
// handler here shows up only as a late tick — which is exactly how a set of
// full-table storage scans went unnoticed until they were costing minutes.
func (e *Engine) timeEvent(event string, fn func()) {
	start := time.Now()
	fn()
	e.metrics.ObserveDispatchEvent(event, time.Since(start).Seconds())
}

func (e *Engine) dispatch(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticks:
			e.timeEvent("tick", func() { e.onTick(ctx) })

		case <-e.earlyAggregateCh:
			e.timeEvent("early_aggregate", func() { e.maybeEarlyAggregate(e.nowMs()) })

		case block := <-e.blockCh:
			e.timeEvent("block", func() { e.onBlock(block) })

		case result := <-e.proposalResultCh:
			e.timeEvent("proposal_result", func() { e.acceptProposal(ctx, result) })

		case root := <-e.failedRootCh:
			e.timeEvent("failed_root", func() { e.onFailedRoot(root) })

		case att := <-e.verifiedAttestationCh:
			e.addVerifiedAttestation(att)

		case p := <-e.newPayloadCh:
			e.addNewPayload(p)

		case result := <-e.aggregationResultCh:
			e.timeEvent("aggregation_result", func() { result.Apply(e.store) })
		}
	}
}
