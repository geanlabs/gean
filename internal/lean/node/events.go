package node

import (
	"context"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/metrics"
	"github.com/geanlabs/gean/internal/lean/types"
)

func (e *Engine) OnBlock(block *types.SignedBlock) {
	e.noteGossipSlot(block)
	e.Importer.SubmitGossip(block)
}

// noteGossipSlot records the highest slot heard on gossip as evidence that the
// network is producing blocks — counted before admission, because a node that
// drops or rejects what it hears must not mistake its own import stall for a
// network-wide one. Far-future slots are ignored so a hostile peer cannot pin
// the duty gate shut with a fabricated slot.
func (e *Engine) noteGossipSlot(block *types.SignedBlock) {
	if block == nil || block.Block == nil {
		return
	}
	slot := block.Block.Slot
	if slot > e.currentSlot(clock.UnixMilli(e.Clock))+1 {
		return
	}
	for {
		cur := e.maxSeenGossipSlot.Load()
		if slot <= cur || e.maxSeenGossipSlot.CompareAndSwap(cur, slot) {
			return
		}
	}
}

// OnSyncBlock delivers a block this node itself requested; see
// importer.Importer.SubmitSync.
func (e *Engine) OnSyncBlock(ctx context.Context, block *types.SignedBlock) bool {
	return e.Importer.SubmitSync(ctx, block)
}

func (e *Engine) OnGossipAttestation(att *types.SignedAttestation) {
	select {
	case e.AttestationCh <- att:
	default:
		// One log line per drop would turn a flood into a log flood; the
		// chain-status tick reports the count once per slot instead.
		metrics.IncAttestationsDropped()
		e.attestationsDropped.Add(1)
	}
}

func (e *Engine) OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	select {
	case e.AggregationCh <- agg:
	default:
		logger.Warn(logger.Signature, "aggregation channel full, dropping")
	}
}
