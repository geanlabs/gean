package node

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// onTick processes an 800ms tick event.
func (e *Engine) onTick() {
	timestampMs := uint64(time.Now().UnixMilli())

	currentSlot := e.currentSlot(timestampMs)
	currentInterval := e.currentInterval(timestampMs)

	SetCurrentSlot(currentSlot)

	// Check if we're the proposer for this slot.
	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	// Tick the store — handles interval dispatch (promote attestations, aggregate).
	newAggregates := OnTick(e.Store, timestampMs, hasProposal, e.IsAggregator)

	// Publish new aggregates from interval 2.
	for _, agg := range newAggregates {
		if e.P2P != nil {
			e.P2P.PublishAggregatedAttestation(context.Background(), agg)
		}
	}

	// Interval 0: propose block if we're the proposer.
	if hasProposal {
		e.maybePropose(currentSlot, proposerValidatorID)
	}

	// Interval 0/4: update head after attestation promotion.
	if currentInterval == 0 || currentInterval == 4 {
		e.updateHead(false)
	}

	// Interval 1: produce attestations + chain status log.
	if currentInterval == 1 {
		e.produceAttestations(currentSlot)
		e.logChainStatus(currentSlot)
	}

	// Interval 3: update safe target.
	if currentInterval == 3 {
		e.updateSafeTarget()
	}
}

// updateHead runs LMD GHOST using known attestations.
func (e *Engine) updateHead(logTree bool) {
	attestations := e.Store.ExtractLatestKnownAttestations()
	justifiedRoot := e.Store.LatestJustified().Root

	// Feed attestations to fork choice vote store.
	for vid, data := range attestations {
		idx := e.FC.NodeIndex(data.Head.Root)
		if idx >= 0 {
			e.FC.Votes.SetKnown(vid, idx, data.Slot, data)
		}
	}

	oldHead := e.Store.Head()
	newHead := e.FC.UpdateHead(justifiedRoot)

	if newHead != oldHead {
		e.Store.SetHead(newHead)
		if !types.IsZeroRoot(oldHead) {
			newHeader := e.Store.GetBlockHeader(newHead)
			justified := e.Store.LatestJustified()
			finalized := e.Store.LatestFinalized()

			// Check if this is a real reorg (new head's parent != old head)
			// or normal chain extension (new head is child of old head).
			isReorg := newHeader != nil && newHeader.ParentRoot != oldHead

			SetHeadSlot(newHeader.Slot)
			SetLatestJustifiedSlot(justified.Slot)
			SetLatestFinalizedSlot(finalized.Slot)
			SetGossipSignatures(e.Store.GossipSignatures.Len())
			SetNewAggregatedPayloads(e.Store.NewPayloads.Len())
			SetKnownAggregatedPayloads(e.Store.KnownPayloads.Len())

			if isReorg {
				IncForkChoiceReorgs()
				logger.Warn(logger.Forkchoice, "REORG slot=%d head_root=0x%x parent_root=0x%x (was 0x%x) justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					newHeader.Slot, newHead, newHeader.ParentRoot, oldHead,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			} else if newHeader != nil {
				logger.Info(logger.Forkchoice, "head slot=%d head_root=0x%x parent_root=0x%x justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					newHeader.Slot, newHead, newHeader.ParentRoot,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			}
		}
	}
}

// updateSafeTarget runs LMD GHOST with 2/3 threshold using all attestations.
func (e *Engine) updateSafeTarget() {
	attestations := e.Store.ExtractLatestAllAttestations()
	justifiedRoot := e.Store.LatestJustified().Root

	// Feed merged attestations to vote store as "new" for safe target.
	for vid, data := range attestations {
		idx := e.FC.NodeIndex(data.Head.Root)
		if idx >= 0 {
			e.FC.Votes.SetNew(vid, idx, data.Slot, data)
		}
	}

	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return
	}
	numValidators := uint64(len(headState.Validators))

	safeTarget := e.FC.UpdateSafeTarget(justifiedRoot, numValidators)
	e.Store.SetSafeTarget(safeTarget)

	safeHeader := e.Store.GetBlockHeader(safeTarget)
	if safeHeader != nil {
		SetSafeTargetSlot(safeHeader.Slot)
	}
}

// logChainStatus prints a chain status summary every slot at interval 1.

func (e *Engine) logChainStatus(currentSlot uint64) {
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
	if e.P2P != nil {
		peerCount = e.P2P.ConnectedPeers()
	}

	gossipSigs := e.Store.GossipSignatures.Len()
	newPayloads := e.Store.NewPayloads.Len()
	knownPayloads := e.Store.KnownPayloads.Len()

	// Build mesh info string with full topic paths.
	meshInfo := ""
	if e.P2P != nil {
		meshSizes := e.P2P.TopicMeshSizes()
		for topic, size := range meshSizes {
			meshInfo += fmt.Sprintf("\n  %-60s mesh_peers=%d", topic, size)
		}
	}

	logger.Info(logger.Chain, "\n\n+===============================================================+\n  CHAIN STATUS: Current Slot: %d | Head Slot: %d | Behind: %d\n+---------------------------------------------------------------+\n  Connected Peers:    %d\n+---------------------------------------------------------------+\n  Head Block Root:    0x%x\n  Parent Block Root:  0x%x\n  State Root:         0x%x\n+---------------------------------------------------------------+\n  Latest Justified:   Slot %6d | Root: 0x%x\n  Latest Finalized:   Slot %6d | Root: 0x%x\n+---------------------------------------------------------------+\n  Gossip Sigs: %d | New Payloads: %d | Known Payloads: %d\n+---------------------------------------------------------------+\n  Topics:%s\n+===============================================================+\n",
		currentSlot, headSlot, behind,
		peerCount,
		headRoot, parentRoot, stateRoot,
		justified.Slot, justified.Root,
		finalized.Slot, finalized.Root,
		gossipSigs, newPayloads, knownPayloads,
		meshInfo)
}

// currentSlot derives the current slot from a timestamp.
func (e *Engine) currentSlot(timestampMs uint64) uint64 {
	genesisMs := e.Store.Config().GenesisTime * 1000
	if timestampMs < genesisMs {
		return 0
	}
	return (timestampMs - genesisMs) / types.MillisecondsPerSlot
}

// currentInterval derives the current interval within a slot.
func (e *Engine) currentInterval(timestampMs uint64) uint64 {
	genesisMs := e.Store.Config().GenesisTime * 1000
	if timestampMs < genesisMs {
		return 0
	}
	totalIntervals := (timestampMs - genesisMs) / types.MillisecondsPerInterval
	return totalIntervals % types.IntervalsPerSlot
}

// getOurProposer checks if any of our validators is the proposer for this slot.
func (e *Engine) getOurProposer(slot uint64) (uint64, bool) {
	if e.Keys == nil {
		return 0, false
	}
	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return 0, false
	}
	numValidators := headState.NumValidators()

	for _, vid := range e.Keys.ValidatorIDs() {
		if types.IsProposer(slot, vid, numValidators) {
			return vid, true
		}
	}
	return 0, false
}
