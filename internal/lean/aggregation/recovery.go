package aggregation

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/attestationproof"
	"github.com/geanlabs/gean/internal/lean/metrics"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

const maxRecoverySplits = 4

const (
	// recoverySplitBudget is how long one Type-2 split is assumed to hold the prover.
	// Measured at roughly 700ms, rounded up to a full interval so the estimate errs
	// toward yielding.
	recoverySplitBudget = types.MillisecondsPerInterval * time.Millisecond
	// aggregationDispatchOffset is how far into a slot aggregation is dispatched;
	// onTick fires it at interval 2.
	aggregationDispatchOffset = 2 * types.MillisecondsPerInterval * time.Millisecond
)

// splitFitsBeforeAggregation reports whether a Type-2 split started now would be done
// with the prover before aggregation next needs it. A split takes about as long as
// aggregation is willing to wait for the gate, so one running across the dispatch costs
// the slot its aggregate — and a missed aggregate slows justification, the very thing
// recovery exists to help. Recovery is best-effort and the aggregate is duty work, so
// recovery yields the window rather than racing for it.
func (a *Aggregator) splitFitsBeforeAggregation(nowMs uint64) bool {
	// A position within the slot never reaches a slot's length.
	intoSlot := time.Duration(min(a.msIntoSlot(nowMs), types.MillisecondsPerSlot)) * time.Millisecond
	windowEnd := aggregationDispatchOffset + SessionBudget
	if intoSlot >= aggregationDispatchOffset && intoSlot < windowEnd {
		return false
	}
	untilDispatch := aggregationDispatchOffset - intoSlot
	if intoSlot >= windowEnd {
		// Past this slot's session; the next dispatch is in the following slot.
		untilDispatch = types.MillisecondsPerSlot*time.Millisecond - intoSlot + aggregationDispatchOffset
	}
	return untilDispatch >= recoverySplitBudget
}

type recoveryCandidate struct {
	att      *types.AggregatedAttestation
	root     [32]byte
	newCount int
}

func (a *Aggregator) DispatchRecovery(block *types.SignedBlock) {
	if block == nil || block.Block == nil || block.Block.Body == nil ||
		len(block.Block.Body.Attestations) == 0 {
		return
	}
	select {
	case a.recovery <- block:
		metrics.SetProvingQueueDepth("recovery", len(a.recovery))
	default:
		metrics.IncProofOperation("recovery", "canceled")
	}
}

func (a *Aggregator) RunRecovery(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-a.recovery:
			metrics.SetProvingQueueDepth("recovery", len(a.recovery))
			a.recoverBlockProofs(ctx, block)
		}
	}
}

func (a *Aggregator) recoverBlockProofs(ctx context.Context, signedBlock *types.SignedBlock) {
	// Only a synced aggregator deconstructs blocks: it re-broadcasts the recovered
	// proofs, while non-aggregators rely on the gossip path. Recovery is skipped while
	// syncing, when historical blocks flood this path and the justified anchor is still
	// moving, so recovered votes would not match a live head.
	if !a.isAggregator() || !a.synced() ||
		signedBlock == nil || signedBlock.Block == nil || signedBlock.Block.Body == nil ||
		signedBlock.Proof == nil || len(signedBlock.Proof.Proof) == 0 {
		return
	}
	now := clock.UnixMilli(a.cfg.Clock)
	currentSlot := a.slotAt(now)
	if a.proposerAt(currentSlot + 1) {
		return
	}

	block := signedBlock.Block
	state := a.cfg.Store.GetState(block.ParentRoot)
	if state == nil {
		return
	}
	// The head post-state's justified checkpoint is the source selectRecoveryCandidates
	// filters votes against.
	headSummary, ok := a.cfg.Store.StateSummary(a.cfg.Store.Head())
	if !ok {
		return
	}
	pubkeys, bindings, err := a.blockProofClaims(block, state)
	if err != nil {
		return
	}

	newEntries := a.cfg.Store.NewPayloads.Entries()
	knownEntries := a.cfg.Store.KnownPayloads.Entries()
	candidates := selectRecoveryCandidates(block.Body.Attestations, &headSummary.Justified, newEntries, knownEntries)

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return
		}
		now = clock.UnixMilli(a.cfg.Clock)
		currentSlot = a.slotAt(now)
		if a.proposerAt(currentSlot + 1) {
			return
		}
		if !a.splitFitsBeforeAggregation(now) {
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		if a.cfg.Gate != nil && !a.cfg.Gate.Acquire(ctx, false) {
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		// Acquire blocks for as long as the current holder keeps the prover, so the
		// check above describes when recovery asked, not when it was handed over.
		// Sessions routinely run past their nominal budget, so re-check before
		// spending the gate and give it back if the window closed while waiting.
		if !a.splitFitsBeforeAggregation(clock.UnixMilli(a.cfg.Clock)) {
			if a.cfg.Gate != nil {
				a.cfg.Gate.Release(false)
			}
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		started := time.Now()
		proof, err := xmss.SplitType2Proof(signedBlock.Proof.Proof, pubkeys, bindings, candidate.root)
		var recovered *types.SingleMessageAggregate
		if err == nil {
			recovered = &types.SingleMessageAggregate{
				Participants: append([]byte(nil), candidate.att.AggregationBits...),
				Proof:        proof,
			}
			locals := append(localProofs(newEntries[candidate.root]), localProofs(knownEntries[candidate.root])...)
			if len(locals) > 0 {
				_, combined, _, mergeErr := attestationproof.Select(
					candidate.att.Data,
					append([]*types.SingleMessageAggregate{recovered}, locals...),
					state,
					attestationproof.NewMerger(a.cfg.Store.PubKeyCache),
				)
				if mergeErr == nil && coversParticipants(combined, candidate.att.AggregationBits) {
					recovered = combined
				}
			}
		}
		if a.cfg.Gate != nil {
			a.cfg.Gate.Release(false)
		}
		metrics.ObserveProvingDuration("recovery", time.Since(started).Seconds())
		if err != nil {
			metrics.IncProofOperation("recovery", "error")
		} else {
			metrics.IncProofOperation("recovery", "success")
			metrics.ObserveProofSize("type1", len(proof))
			a.cfg.Store.NewPayloads.Push(candidate.root, candidate.att.Data, recovered)
			if a.cfg.Publisher != nil {
				_ = a.cfg.Publisher.PublishAggregatedAttestation(ctx, &types.SignedAggregatedAttestation{
					Data:  candidate.att.Data,
					Proof: recovered,
				})
			}
		}
	}
}

// selectRecoveryCandidates picks the block attestations worth splitting back into
// per-attestation proofs. Only votes whose source equals the head's justified
// checkpoint can be packed into a future block on this head, so any other source is
// dropped. Attestations that add no participants beyond the locally-held proofs are
// skipped, and the rest are ranked by new participants and capped to bound proving work.
func selectRecoveryCandidates(
	attestations []*types.AggregatedAttestation,
	headJustified *types.Checkpoint,
	newEntries, knownEntries map[[32]byte]*store.PayloadEntry,
) []recoveryCandidate {
	candidates := make([]recoveryCandidate, 0, len(attestations))
	for _, att := range attestations {
		if att == nil || att.Data == nil || att.Data.Source == nil || *att.Data.Source != *headJustified {
			continue
		}
		root, err := att.Data.HashTreeRoot()
		if err != nil {
			continue
		}
		covered := localCoverage(newEntries[root], knownEntries[root])
		newCount := 0
		for _, index := range types.BitlistIndices(att.AggregationBits) {
			if !covered[index] {
				newCount++
			}
		}
		if newCount > 0 {
			candidates = append(candidates, recoveryCandidate{att: att, root: root, newCount: newCount})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].newCount > candidates[j].newCount
	})
	if len(candidates) > maxRecoverySplits {
		candidates = candidates[:maxRecoverySplits]
	}
	return candidates
}

func coversParticipants(proof *types.SingleMessageAggregate, participants []byte) bool {
	return proof != nil && types.BitlistIsSubset(proof.Participants, participants)
}

// blockProofClaims returns the signer groups of a block's Type-2 proof and what each
// signed, in the order block verification binds them: one group per attestation, then
// the proposer over the block root.
func (a *Aggregator) blockProofClaims(block *types.Block, state *types.State) ([][]xmss.CPubKey, []xmss.MessageBinding, error) {
	groups := make([][]xmss.CPubKey, 0, len(block.Body.Attestations)+1)
	bindings := make([]xmss.MessageBinding, 0, len(block.Body.Attestations)+1)
	for _, att := range block.Body.Attestations {
		keys := make([]xmss.CPubKey, 0, types.BitlistCount(att.AggregationBits))
		for _, index := range types.BitlistIndices(att.AggregationBits) {
			if index >= uint64(len(state.Validators)) || state.Validators[index] == nil {
				return nil, nil, fmt.Errorf("validator %d out of range", index)
			}
			key, err := a.cfg.Store.PubKeyCache.Get(state.Validators[index].AttestationPubkey)
			if err != nil {
				return nil, nil, err
			}
			keys = append(keys, key)
		}
		root, err := att.Data.HashTreeRoot()
		if err != nil {
			return nil, nil, err
		}
		groups = append(groups, keys)
		bindings = append(bindings, xmss.MessageBinding{Message: root, Slot: att.Data.Slot})
	}
	if block.ProposerIndex >= uint64(len(state.Validators)) || state.Validators[block.ProposerIndex] == nil {
		return nil, nil, fmt.Errorf("proposer %d out of range", block.ProposerIndex)
	}
	key, err := a.cfg.Store.PubKeyCache.Get(state.Validators[block.ProposerIndex].ProposalPubkey)
	if err != nil {
		return nil, nil, err
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	groups = append(groups, []xmss.CPubKey{key})
	bindings = append(bindings, xmss.MessageBinding{Message: blockRoot, Slot: block.Slot})
	return groups, bindings, nil
}

func localCoverage(entries ...*store.PayloadEntry) map[uint64]bool {
	covered := make(map[uint64]bool)
	for _, entry := range entries {
		for _, proof := range localProofs(entry) {
			for _, index := range types.BitlistIndices(proof.Participants) {
				covered[index] = true
			}
		}
	}
	return covered
}

func localProofs(entry *store.PayloadEntry) []*types.SingleMessageAggregate {
	if entry == nil {
		return nil
	}
	return entry.Proofs
}

func (a *Aggregator) synced() bool {
	return a.cfg.Synced != nil && a.cfg.Synced()
}

func (a *Aggregator) proposerAt(slot uint64) bool {
	return a.cfg.ProposerAt != nil && a.cfg.ProposerAt(slot)
}
