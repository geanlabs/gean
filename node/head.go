package node

import (
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) updateHead() {
	defer e.publishForkChoiceView()
	attestations := e.store.ExtractLatestKnownAttestations()
	justifiedRoot := e.store.LatestJustified().Root

	// FindHead echoes back a justified root it doesn't know, freezing the
	// head there — surface that state instead of stalling silently.
	if e.forkChoice.NodeIndex(justifiedRoot) < 0 && e.warnedMissingJustified != justifiedRoot {
		e.warnedMissingJustified = justifiedRoot
		logger.Warn(logger.Forkchoice, "justified root 0x%x unknown to fork choice; head cannot advance past it", justifiedRoot)
	}

	for vid, data := range attestations {
		e.forkChoice.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}

	oldHead := e.store.Head()
	newHead := e.forkChoice.UpdateHead(justifiedRoot)

	e.updateFinalizedFromHead(newHead)

	if newHead != oldHead {
		e.store.SetHead(newHead)
		if !types.IsZeroRoot(oldHead) {
			newHeader := e.store.GetBlockHeader(newHead)
			if newHeader == nil {
				return
			}
			justified := e.store.LatestJustified()
			finalized := e.store.LatestFinalized()

			isReorg := newHeader.ParentRoot != oldHead

			metrics.SetHeadSlot(newHeader.Slot)
			metrics.SetLatestJustifiedSlot(justified.Slot)
			metrics.SetLatestFinalizedSlot(finalized.Slot)
			metrics.SetJustifiedSlot(justified.Slot)
			metrics.SetFinalizedSlot(finalized.Slot)
			metrics.SetGossipSignatures(e.store.AttestationSignatures().Len())
			metrics.SetNewAggregatedPayloads(e.store.NewPayloads().Len())
			metrics.SetKnownAggregatedPayloads(e.store.KnownPayloads().Len())
			metrics.SetPendingAttestationsTotal(e.pendingAttestations.Total())

			if isReorg {
				metrics.IncForkChoiceReorgs()
				depth := e.forkChoice.ReorgDepth(oldHead, newHead)
				metrics.ObserveForkChoiceReorgDepth(float64(depth))
				logger.Warn(logger.Forkchoice, "REORG depth=%d slot=%d head_root=0x%x parent_root=0x%x (was 0x%x) justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					depth, newHeader.Slot, newHead, newHeader.ParentRoot, oldHead,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			} else {
				logger.Info(logger.Forkchoice, "head slot=%d head_root=0x%x parent_root=0x%x justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					newHeader.Slot, newHead, newHeader.ParentRoot,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			}
		}
	}
}

// updateFinalizedFromHead keeps the finalized checkpoint on the canonical head's
// chain. The finalized slot is taken from the head's post-state and re-anchored to
// the head's ancestor at that slot, rather than advanced independently per imported
// block: a losing fork that finalizes a higher slot must not latch finalization
// above the canonical head, which would otherwise stall target advancement. When
// finalization advances it drives the same pruning/discard work the import path used.
func (e *Engine) updateFinalizedFromHead(headRoot [32]byte) {
	derived := store.DeriveFinalizedFromHead(e.store, headRoot)
	if derived == nil {
		return
	}

	old := e.store.LatestFinalized()
	oldSlot := uint64(0)
	if old != nil {
		if derived.Root == old.Root && derived.Slot == old.Slot {
			return
		}
		oldSlot = old.Slot
	}

	// Set unconditionally to the canonical head's finalized checkpoint, not a
	// running maximum: a higher-finalized fork that loses head selection must not
	// latch finalization above the head, so the checkpoint moves down when the head
	// reorgs onto a chain that finalized fewer slots.
	e.store.SetLatestFinalized(derived)

	// Pruning is irreversible, so it only runs when finalization genuinely
	// advances; a downward move keeps the existing pruned horizon.
	if derived.Slot > oldSlot {
		metrics.IncFinalization("success")
		logger.Info(logger.Forkchoice, "finalized advanced slot=%d root=0x%x", derived.Slot, derived.Root)
		// Order matters and is not interchangeable. PruneOnFinalization asks fork
		// choice which roots to delete (GetCanonicalAnalysis: the ancestors below
		// the new finalized root, and every branch that is neither ancestor nor
		// descendant of it). FC.Prune removes exactly those nodes from the
		// ProtoArray. Pruning fork choice first therefore leaves the analysis with
		// nothing to report — canonical is length 1 so canonical[1:] is empty, and
		// nonCanonical is empty — so the database prune silently deletes nothing
		// and TableStates/TableBlockHeaders grow for the life of the chain.
		store.PruneOnFinalization(e.store, e.forkChoice, oldSlot, derived.Slot, derived.Root)
		if derived.Slot > 0 {
			e.forkChoice.Prune(derived.Root)
		}
		e.discardFinalizedPending(derived.Slot)
	}
}

func (e *Engine) updateSafeTarget() {
	defer e.publishForkChoiceView()
	attestations := e.store.ExtractLatestNewAttestations()
	justifiedRoot := e.store.LatestJustified().Root

	for vid, data := range attestations {
		e.forkChoice.SetNewVote(vid, data.Head.Root, data.Slot, data)
	}

	headState := e.store.GetState(e.store.Head())
	if headState == nil {
		return
	}
	numValidators := uint64(len(headState.Validators))

	safeTarget := e.forkChoice.UpdateSafeTarget(justifiedRoot, numValidators)
	e.store.SetSafeTarget(safeTarget)

	safeHeader := e.store.GetBlockHeader(safeTarget)
	if safeHeader != nil {
		metrics.SetSafeTargetSlot(safeHeader.Slot)
	}
}

// ForkChoiceView returns the fork choice view published after the latest head
// or safe-target update. It is safe to call from any goroutine.
func (e *Engine) ForkChoiceView() *forkchoice.View {
	return e.forkChoiceView.Load()
}

func (e *Engine) publishForkChoiceView() {
	// A block's proposer never changes, so carry it over from the previous
	// view and read headers only for nodes added since.
	known := make(map[[32]byte]uint64)
	if prev := e.forkChoiceView.Load(); prev != nil {
		for _, n := range prev.Nodes {
			known[n.Root] = n.ProposerIndex
		}
	}
	protoNodes := e.forkChoice.Nodes()
	nodes := make([]forkchoice.ViewNode, len(protoNodes))
	for i, pn := range protoNodes {
		proposer, ok := known[pn.Root]
		if !ok {
			if header := e.store.GetBlockHeader(pn.Root); header != nil {
				proposer = header.ProposerIndex
			}
		}
		nodes[i] = forkchoice.ViewNode{ProtoNode: pn, ProposerIndex: proposer}
	}
	e.forkChoiceView.Store(&forkchoice.View{
		Nodes:          nodes,
		Head:           e.store.Head(),
		Justified:      *e.store.LatestJustified(),
		Finalized:      *e.store.LatestFinalized(),
		SafeTarget:     e.store.SafeTarget(),
		ValidatorCount: e.validatorCount(),
	})
}
