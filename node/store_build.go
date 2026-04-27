package node

import (
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// ProduceBlockWithSignatures builds a block using per-AttestationData fixed-point selection.
// Returns the block and per-attestation signature proofs.
// Spec: lean_spec/subspecs/containers/state/state.py build_block
func ProduceBlockWithSignatures(
	s *ConsensusStore,
	slot, validatorIndex uint64,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	buildStart := time.Now()

	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil {
		IncBlockBuildingFailures()
		return nil, nil, &StoreError{ErrMissingParentState,
			fmt.Sprintf("head state missing for slot %d", slot)}
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		IncBlockBuildingFailures()
		return nil, nil, errNotProposer(validatorIndex, slot)
	}

	// Flush any pending NewPayloads → KnownPayloads before building.
	// Matches leanSpec get_proposal_head which calls accept_new_attestations
	// right before reading latest_known_aggregated_payloads.
	s.PromoteNewToKnown()

	storeJustified := s.LatestJustified()

	knownEntries := s.KnownPayloads.Entries()
	knownBlockRoots := s.getBlockRoots()

	block, sigs, err := buildBlock(headState, slot, validatorIndex, headRoot, knownBlockRoots, knownEntries, storeJustified, s.PubKeyCache)

	ObserveBlockBuildingTime(time.Since(buildStart).Seconds())
	if err != nil {
		IncBlockBuildingFailures()
		return nil, nil, err
	}
	IncBlockBuildingSuccess()
	if block != nil && block.Body != nil {
		ObserveBlockAggregatedPayloads(len(block.Body.Attestations))
	}
	return block, sigs, nil
}

// buildBlock builds a valid block using per-AttestationData fixed-point selection
// with greedy proof coverage and MAX_ATTESTATIONS_DATA cap.
//
// Algorithm (per spec build_block):
//  1. Sort payloads by target.slot for deterministic order
//  2. For each AttestationData whose source == current_justified:
//     a. Skip if head not in known_block_roots
//     b. Skip if already processed
//     c. Greedy proof selection: pick proofs maximizing new validator coverage
//  3. Trial STF — if justified advances, update source and continue
//  4. Enforce MAX_ATTESTATIONS_DATA cap
func buildBlock(
	headState *types.State,
	slot, proposerIndex uint64,
	parentRoot [32]byte,
	knownBlockRoots map[[32]byte]bool,
	payloads map[[32]byte]*PayloadEntry,
	storeJustified *types.Checkpoint,
	pkCache *xmss.PubKeyCache,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	if len(payloads) > 0 {
		aggStart := time.Now()
		defer func() { ObserveBlockBuildingPayloadAggregationTime(time.Since(aggStart).Seconds()) }()
		// Use store justified (stable, converges via tiebreak).
		// Both attestation source and builder use store justified,
		// which converges across nodes via deterministic root tiebreak.
		var currentJustified *types.Checkpoint
		if headState.LatestBlockHeader.Slot == 0 {
			currentJustified = &types.Checkpoint{
				Root: parentRoot,
				Slot: storeJustified.Slot,
			}
		} else {
			currentJustified = storeJustified
		}

		logger.Info(logger.Chain, "buildBlock: currentJustified root=0x%x slot=%d",
			currentJustified.Root, currentJustified.Slot)

		// Sort payloads by target.slot for deterministic processing order.
		type payloadItem struct {
			dataRoot [32]byte
			entry    *PayloadEntry
		}
		sorted := make([]payloadItem, 0, len(payloads))
		for dr, entry := range payloads {
			sorted = append(sorted, payloadItem{dataRoot: dr, entry: entry})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].entry.Data.Target.Slot != sorted[j].entry.Data.Target.Slot {
				return sorted[i].entry.Data.Target.Slot < sorted[j].entry.Data.Target.Slot
			}
			return compareRoots(sorted[i].dataRoot, sorted[j].dataRoot) < 0
		})

		processedAttData := make(map[[32]byte]bool)

		for {
			foundEntries := false

			for _, item := range sorted {
				// MAX_ATTESTATIONS_DATA cap on build side.
				if len(processedAttData) >= int(types.MaxAttestationsData) {
					break
				}

				if processedAttData[item.dataRoot] {
					continue
				}
				if !knownBlockRoots[item.entry.Data.Head.Root] {
					logger.Info(logger.Chain, "buildBlock: SKIP unknown head root=0x%x attSlot=%d",
						item.entry.Data.Head.Root, item.entry.Data.Slot)
					continue
				}
				if item.entry.Data.Source.Root != currentJustified.Root ||
					item.entry.Data.Source.Slot != currentJustified.Slot {
					continue
				}

				processedAttData[item.dataRoot] = true
				foundEntries = true

				// Greedy set-cover: select proofs maximizing validator coverage.
				// If multiple proofs selected, merge via recursive aggregation.
				selectGreedyProofs(item.entry, headState, pkCache, &attestations, &signatures)
			}

			if !foundEntries {
				break
			}

			// Check if justification advanced via trial state transition.
			candidate := &types.Block{
				Slot:          slot,
				ProposerIndex: proposerIndex,
				ParentRoot:    parentRoot,
				Body:          &types.BlockBody{Attestations: attestations},
			}
			trialBytes, _ := headState.MarshalSSZ()
			trialState := &types.State{}
			trialState.UnmarshalSSZ(trialBytes)

			statetransition.ProcessSlots(trialState, slot)
			statetransition.ProcessBlock(trialState, candidate)

			if trialState.LatestJustified.Slot != currentJustified.Slot ||
				trialState.LatestJustified.Root != currentJustified.Root {
				currentJustified = trialState.LatestJustified
				continue
			}

			break
		}
	}

	// Build final block with correct state root.
	finalBlock := &types.Block{
		Slot:          slot,
		ProposerIndex: proposerIndex,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{Attestations: attestations},
	}

	finalBytes, _ := headState.MarshalSSZ()
	postState := &types.State{}
	postState.UnmarshalSSZ(finalBytes)

	if err := statetransition.ProcessSlots(postState, slot); err != nil {
		return nil, nil, fmt.Errorf("process slots: %w", err)
	}
	if err := statetransition.ProcessBlock(postState, finalBlock); err != nil {
		return nil, nil, fmt.Errorf("process block: %w", err)
	}

	stateRoot, _ := postState.HashTreeRoot()
	finalBlock.StateRoot = stateRoot

	// Spec assertion: the fixed-point loop must close any justified divergence.
	// The produced block's post-state justified must be >= store justified.
	if postState.LatestJustified.Slot < storeJustified.Slot {
		logger.Error(logger.Chain, "buildBlock: justified divergence not closed: block_justified=%d < store_justified=%d",
			postState.LatestJustified.Slot, storeJustified.Slot)
	}

	return finalBlock, signatures, nil
}

// selectGreedyProofs uses greedy set-cover to select proofs maximizing validator
// coverage for a single AttestationData. If multiple proofs are selected, they
// are merged via recursive aggregation (AggregateWithChildren) into a single
// compound proof — producing one attestation per AttestationData in the block.
//
// Spec: select_greedily + AggregatedSignatureProof.aggregate(children=...)
func selectGreedyProofs(
	entry *PayloadEntry,
	state *types.State,
	pkCache *xmss.PubKeyCache,
	attestations *[]*types.AggregatedAttestation,
	signatures *[]*types.AggregatedSignatureProof,
) {
	if len(entry.Proofs) == 0 {
		return
	}

	// Single proof: use directly, no merging needed.
	if len(entry.Proofs) == 1 {
		proof := entry.Proofs[0]
		if countParticipants(proof.Participants) == 0 {
			return
		}
		*attestations = append(*attestations, &types.AggregatedAttestation{
			AggregationBits: proof.Participants,
			Data:            entry.Data,
		})
		*signatures = append(*signatures, proof)
		return
	}

	// Greedy set-cover: iteratively pick the proof covering the most
	// uncovered validators until no new coverage is gained.
	covered := make(map[uint64]bool)
	remaining := make(map[int]bool, len(entry.Proofs))
	for i := range entry.Proofs {
		remaining[i] = true
	}

	var selected []*types.AggregatedSignatureProof
	for len(remaining) > 0 {
		bestIdx := -1
		bestNew := 0

		for idx := range remaining {
			newCount := 0
			bitsLen := types.BitlistLen(entry.Proofs[idx].Participants)
			for vid := uint64(0); vid < bitsLen; vid++ {
				if types.BitlistGet(entry.Proofs[idx].Participants, vid) && !covered[vid] {
					newCount++
				}
			}
			if newCount > bestNew {
				bestNew = newCount
				bestIdx = idx
			}
		}

		if bestIdx < 0 || bestNew == 0 {
			break
		}

		proof := entry.Proofs[bestIdx]
		selected = append(selected, proof)
		delete(remaining, bestIdx)

		// Mark covered validators.
		bitsLen := types.BitlistLen(proof.Participants)
		for vid := uint64(0); vid < bitsLen; vid++ {
			if types.BitlistGet(proof.Participants, vid) {
				covered[vid] = true
			}
		}
	}

	if len(selected) == 0 {
		return
	}

	// Single proof selected: use directly.
	if len(selected) == 1 {
		*attestations = append(*attestations, &types.AggregatedAttestation{
			AggregationBits: selected[0].Participants,
			Data:            entry.Data,
		})
		*signatures = append(*signatures, selected[0])
		return
	}

	// Multiple proofs: merge via recursive aggregation.
	merged := mergeProofs(selected, entry.Data, state, pkCache)
	if merged == nil {
		return // skip this AttestationData if merge fails
	}
	*attestations = append(*attestations, &types.AggregatedAttestation{
		AggregationBits: merged.Participants,
		Data:            entry.Data,
	})
	*signatures = append(*signatures, merged)
}

// mergeProofs recursively aggregates multiple proofs for the same AttestationData
// into a single compound proof using AggregateWithChildren.
// Returns nil if merging fails (caller should fall back to single best proof).
func mergeProofs(
	proofs []*types.AggregatedSignatureProof,
	attData *types.AttestationData,
	state *types.State,
	pkCache *xmss.PubKeyCache,
) *types.AggregatedSignatureProof {
	if len(proofs) < 2 || state == nil || pkCache == nil {
		return nil
	}

	// Build ChildProof structs for the FFI call.
	children := make([]xmss.ChildProof, 0, len(proofs))
	var allIDs []uint64

	for _, proof := range proofs {
		var pubkeys []xmss.CPubKey
		bitsLen := types.BitlistLen(proof.Participants)
		for vid := uint64(0); vid < bitsLen; vid++ {
			if !types.BitlistGet(proof.Participants, vid) {
				continue
			}
			if int(vid) >= len(state.Validators) {
				continue
			}
			pk, err := pkCache.Get(state.Validators[vid].AttestationPubkey)
			if err != nil {
				logger.Error(logger.Chain, "mergeProofs: pubkey parse failed vid=%d: %v", vid, err)
				return nil
			}
			pubkeys = append(pubkeys, pk)
			allIDs = append(allIDs, vid)
		}

		children = append(children, xmss.ChildProof{
			Pubkeys:   pubkeys,
			ProofData: proof.ProofData,
		})
	}

	dataRootHash, _ := attData.HashTreeRoot()
	slot := uint32(attData.Slot)

	mergeStart := time.Now()
	mergedBytes, err := xmss.AggregateWithChildren(nil, nil, children, dataRootHash, slot)
	mergeDuration := time.Since(mergeStart)

	if err != nil {
		logger.Error(logger.Chain, "mergeProofs: AggregateWithChildren failed slot=%d children=%d duration=%v: %v",
			slot, len(children), mergeDuration, err)
		return nil
	}

	logger.Info(logger.Chain, "mergeProofs: merged %d proofs into 1, slot=%d validators=%d proof=%d bytes duration=%v",
		len(proofs), slot, len(allIDs), len(mergedBytes), mergeDuration)

	participants := AggregationBitsFromIndices(allIDs)
	return &types.AggregatedSignatureProof{
		Participants: participants,
		ProofData:    mergedBytes,
	}
}

// countParticipants returns the number of set bits in a participant bitlist.
func countParticipants(bits []byte) int {
	count := 0
	bitsLen := types.BitlistLen(bits)
	for vid := uint64(0); vid < bitsLen; vid++ {
		if types.BitlistGet(bits, vid) {
			count++
		}
	}
	return count
}

// getBlockRoots returns all known block roots from the store.
func (s *ConsensusStore) getBlockRoots() map[[32]byte]bool {
	roots := make(map[[32]byte]bool)
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return roots
	}
	it, err := rv.PrefixIterator(storage.TableBlockHeaders, nil)
	if err != nil {
		return roots
	}
	defer it.Close()
	for it.Next() {
		var root [32]byte
		copy(root[:], it.Key())
		roots[root] = true
	}
	return roots
}

func compareRoots(a, b [32]byte) int {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
