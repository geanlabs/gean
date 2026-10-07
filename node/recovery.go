package node

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/aggregation"
	"github.com/geanlabs/gean/attestationproof"
	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
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
func (e *Engine) splitFitsBeforeAggregation(nowMs uint64) bool {
	intoSlot := time.Duration(e.millisIntoSlot(nowMs)) * time.Millisecond
	windowEnd := aggregationDispatchOffset + aggregation.SessionBudget
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

func (e *Engine) dispatchRecovery(block *types.SignedBlock) {
	if block == nil || block.Block == nil || block.Block.Body == nil ||
		len(block.Block.Body.Attestations) == 0 || e.recoveryCh == nil {
		return
	}
	select {
	case e.recoveryCh <- block:
		metrics.SetProvingQueueDepth("recovery", len(e.recoveryCh))
	default:
		metrics.IncProofOperation("recovery", "canceled")
	}
}

func (e *Engine) runRecoveryWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-e.recoveryCh:
			metrics.SetProvingQueueDepth("recovery", len(e.recoveryCh))
			e.recoverBlockProofs(ctx, block)
		}
	}
}

func (e *Engine) recoverBlockProofs(ctx context.Context, signedBlock *types.SignedBlock) {
	// Only a synced aggregator deconstructs blocks: it re-broadcasts the recovered
	// proofs, while non-aggregators rely on the gossip path. Recovery is skipped while
	// syncing, when historical blocks flood this path and the justified anchor is still
	// moving, so recovered votes would not match a live head.
	if e.aggregator == nil || !e.aggregator.Get() || e.GetSyncStatus() != types.SyncSynced ||
		signedBlock == nil || signedBlock.Block == nil || signedBlock.Block.Body == nil ||
		signedBlock.Proof == nil || len(signedBlock.Proof.Proof) == 0 {
		return
	}
	now := e.nowMs()
	currentSlot := e.currentSlot(now)
	if _, proposesNext := e.getOurProposer(currentSlot + 1); proposesNext {
		return
	}

	block := signedBlock.Block
	state := e.store.GetState(block.ParentRoot)
	if state == nil {
		return
	}
	// The head post-state's justified checkpoint is the source selectRecoveryCandidates
	// filters votes against.
	headState := e.store.GetState(e.store.Head())
	if headState == nil || headState.LatestJustified == nil {
		return
	}
	pubkeys, err := e.blockProofPubkeys(block, state)
	if err != nil {
		return
	}

	newEntries := e.store.NewPayloads().Entries()
	knownEntries := e.store.KnownPayloads().Entries()
	candidates := selectRecoveryCandidates(block.Body.Attestations, headState.LatestJustified, newEntries, knownEntries)

	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return
		}
		now = e.nowMs()
		currentSlot = e.currentSlot(now)
		if _, proposesNext := e.getOurProposer(currentSlot + 1); proposesNext {
			return
		}
		if !e.splitFitsBeforeAggregation(now) {
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		if e.provingGate != nil && !e.provingGate.Acquire(ctx, false) {
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		// Acquire blocks for as long as the current holder keeps the prover, so the
		// check above describes when recovery asked, not when it was handed over.
		// Sessions routinely run past their nominal budget, so re-check before
		// spending the gate and give it back if the window closed while waiting.
		if !e.splitFitsBeforeAggregation(e.nowMs()) {
			if e.provingGate != nil {
				e.provingGate.Release(false)
			}
			metrics.IncProofOperation("recovery", "canceled")
			return
		}
		started := time.Now()
		proof, err := xmss.SplitType2Proof(signedBlock.Proof.Proof, pubkeys, candidate.root)
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
					aggregation.NewProofMerger(e.pubKeys),
				)
				if mergeErr == nil && coversParticipants(combined, candidate.att.AggregationBits) {
					recovered = combined
				}
			}
		}
		if e.provingGate != nil {
			e.provingGate.Release(false)
		}
		metrics.ObserveProvingDuration("recovery", time.Since(started).Seconds())
		if err != nil {
			metrics.IncProofOperation("recovery", "error")
		} else {
			metrics.IncProofOperation("recovery", "success")
			metrics.ObserveProofSize("type1", len(proof))
			e.store.NewPayloads().Push(candidate.root, candidate.att.Data, recovered)
			if e.network != nil {
				_ = e.network.PublishAggregatedAttestation(ctx, &types.SignedAggregatedAttestation{
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
	if proof == nil {
		return false
	}
	for _, index := range types.BitlistIndices(participants) {
		if !types.BitlistGet(proof.Participants, index) {
			return false
		}
	}
	return true
}

func (e *Engine) blockProofPubkeys(block *types.Block, state *types.State) ([][]xmss.CPubKey, error) {
	groups := make([][]xmss.CPubKey, 0, len(block.Body.Attestations)+1)
	for _, att := range block.Body.Attestations {
		keys := make([]xmss.CPubKey, 0, types.BitlistCount(att.AggregationBits))
		for _, index := range types.BitlistIndices(att.AggregationBits) {
			if index >= uint64(len(state.Validators)) || state.Validators[index] == nil {
				return nil, fmt.Errorf("validator %d out of range", index)
			}
			key, err := e.pubKeys.Get(state.Validators[index].AttestationPubkey)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}
		groups = append(groups, keys)
	}
	if block.ProposerIndex >= uint64(len(state.Validators)) || state.Validators[block.ProposerIndex] == nil {
		return nil, fmt.Errorf("proposer %d out of range", block.ProposerIndex)
	}
	key, err := e.pubKeys.Get(state.Validators[block.ProposerIndex].ProposalPubkey)
	if err != nil {
		return nil, err
	}
	return append(groups, []xmss.CPubKey{key}), nil
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
