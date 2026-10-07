package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
)

// timeEvent records how long one dispatch-loop event took. Every case runs on
// the single goroutine that also keeps the slot clock, so an unattributed slow
// handler here shows up only as a late tick — which is exactly how a set of
// full-table storage scans went unnoticed until they were costing minutes.
func timeEvent(event string, fn func()) {
	start := time.Now()
	fn()
	metrics.ObserveDispatchEvent(event, time.Since(start).Seconds())
}

func (e *Engine) dispatch(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticks:
			timeEvent("tick", e.onTick)

		case <-e.earlyAggregateCh:
			timeEvent("early_aggregate", func() { e.maybeEarlyAggregate(e.nowMs()) })

		case block := <-e.blockCh:
			timeEvent("block", func() { e.onBlock(block) })

		case result := <-e.proposalResultCh:
			timeEvent("proposal_result", func() { e.acceptProposal(ctx, result) })

		case root := <-e.failedRootCh:
			timeEvent("failed_root", func() { e.onFailedRoot(root) })
		}
	}
}
