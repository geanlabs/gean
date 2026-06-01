package blockbuilder

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// ProduceBlockWithSignatures builds a block using per-AttestationData fixed-point selection.
// Returns the block and per-attestation signature proofs.
func ProduceBlockWithSignatures(
	s *store.ConsensusStore,
	slot, validatorIndex uint64,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	buildStart := time.Now()

	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, &store.StoreError{Kind: store.ErrMissingParentState,
			Message: fmt.Sprintf("head state missing for slot %d", slot)}
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		metrics.IncBlockBuildingFailures()
		return nil, nil, errNotProposer(validatorIndex, slot)
	}

	// Flush any pending NewPayloads → KnownPayloads before building, so the
	// build reads latest_known_aggregated_payloads after the promotion.
	s.PromoteNewToKnown()

	storeJustified := s.LatestJustified()

	knownEntries := s.KnownPayloads.Entries()
	knownBlockRoots := getBlockRoots(s)

	block, sigs, err := buildBlock(headState, slot, validatorIndex, headRoot, knownBlockRoots, knownEntries, storeJustified, s.PubKeyCache)

	metrics.ObserveBlockBuildingTime(time.Since(buildStart).Seconds())
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, err
	}
	metrics.IncBlockBuildingSuccess()
	if block != nil && block.Body != nil {
		metrics.ObserveBlockAggregatedPayloads(len(block.Body.Attestations))
	}
	return block, sigs, nil
}

func errNotProposer(vid, slot uint64) error {
	return &store.StoreError{
		Kind:    store.ErrNotProposer,
		Message: fmt.Sprintf("validator %d not proposer for slot %d", vid, slot),
	}
}

func errJustifiedDivergenceNotClosed(blockJustifiedSlot, storeJustifiedSlot uint64) error {
	return &store.StoreError{
		Kind: store.ErrJustifiedDivergenceNotClosed,
		Message: fmt.Sprintf(
			"produced block justified slot %d is behind store justified slot %d; fixed-point attestation loop did not converge",
			blockJustifiedSlot, storeJustifiedSlot,
		),
	}
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
	payloads map[[32]byte]*store.PayloadEntry,
	storeJustified *types.Checkpoint,
	pkCache *xmss.PubKeyCache,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	if len(payloads) > 0 {
		aggStart := time.Now()
		defer func() { metrics.ObserveBlockBuildingPayloadAggregationTime(time.Since(aggStart).Seconds()) }()
		// Filter attestations by the head state's justified checkpoint.
		// The post-block invariant at the end of this function still gates on
		// storeJustified — if process_attestations advances state.LatestJustified
		// past the anchor, the invariant holds; if no usable attestations exist
		// in the pool, the invariant fires with a clear error.
		//
		// At genesis (LatestBlockHeader.Slot == 0), process_block_header
		// rewrites state.LatestJustified.Root to parent_root. Apply that
		// derivation here so attestation sources match what the STF observes
		// post-header.
		var currentJustified *types.Checkpoint
		if headState.LatestBlockHeader.Slot == 0 {
			currentJustified = &types.Checkpoint{
				Root: parentRoot,
				Slot: headState.LatestJustified.Slot,
			}
		} else {
			currentJustified = headState.LatestJustified
		}

		logger.Info(logger.Chain, "buildBlock: currentJustified root=0x%x slot=%d",
			currentJustified.Root, currentJustified.Slot)

		// Sort payloads by target.slot for deterministic processing order.
		type payloadItem struct {
			dataRoot [32]byte
			entry    *store.PayloadEntry
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
				// The strict source==current_justified check is replaced by
				// three filters: source slot must already be justified on this
				// chain; source/target roots must match the canonical chain at
				// those slots; target slot must not already be justified
				// (genesis self-votes excepted for fork-choice bootstrapping).
				finalizedSlot := headState.LatestFinalized.Slot
				// Chain-match runs first: the bounded slot queries below assume
				// source/target slots are within the chain view, which
				// chain-match enforces.
				if !attestationDataMatchesChain(headState, item.entry.Data) {
					continue
				}
				if !statetransition.IsSlotJustified(headState, finalizedSlot, item.entry.Data.Source.Slot) {
					continue
				}
				isGenesisSelfVote := item.entry.Data.Source.Slot == 0 && item.entry.Data.Target.Slot == 0
				if !isGenesisSelfVote && statetransition.IsSlotJustified(headState, finalizedSlot, item.entry.Data.Target.Slot) {
					continue
				}
				processedAttData[item.dataRoot] = true
				foundEntries = true

				// Greedy set-cover: select proofs maximizing validator coverage.
				// If multiple proofs selected, merge via recursive validator.
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

	// Spec invariant: the fixed-point loop must close any justified divergence.
	// The produced block's post-state justified must be >= store justified.
	// Without this, peers processing the block would never see the
	// justification advance, degrading consensus liveness — only nodes that
	// happened to receive the minority fork would know justification moved.
	if postState.LatestJustified.Slot < storeJustified.Slot {
		return nil, nil, errJustifiedDivergenceNotClosed(
			postState.LatestJustified.Slot, storeJustified.Slot)
	}

	return finalBlock, signatures, nil
}

// selectGreedyProofs uses greedy set-cover to select proofs maximizing validator
// coverage for a single AttestationData. If multiple proofs are selected, they
// are merged via recursive aggregation (AggregateWithChildren) into a single
// compound proof — producing one attestation per AttestationData in the block.
func selectGreedyProofs(
	entry *store.PayloadEntry,
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
			for vid := range bitsLen {
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
		for vid := range bitsLen {
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

	// Multiple proofs: merge via recursive validator.
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
		for vid := range bitsLen {
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

	participants := aggregation.AggregationBitsFromIndices(allIDs)
	return &types.AggregatedSignatureProof{
		Participants: participants,
		ProofData:    mergedBytes,
	}
}

// countParticipants returns the number of set bits in a participant bitlist.
func countParticipants(bits []byte) int {
	count := 0
	bitsLen := types.BitlistLen(bits)
	for vid := range bitsLen {
		if types.BitlistGet(bits, vid) {
			count++
		}
	}
	return count
}

// getBlockRoots returns all known block roots from the store.
func getBlockRoots(s *store.ConsensusStore) map[[32]byte]bool {
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

// attestationDataMatchesChain reports whether the attestation's source and
// target roots equal the canonical block roots recorded for those slots in
// the state's historical block hashes. Empty slots carry a zero entry and
// are rejected; an attestation referencing one cannot be on the canonical
// chain.
//
// Note: this uses state.HistoricalBlockHashes as-is rather than the
// "extended" array that the spec constructs (parent_root
// at parent.slot plus zeros for intermediate empty slots). Attestations
// whose source or target reference the parent slot or later are skipped
// here; the trial state transition still re-runs after each loop pass and
// would surface any missed advances on subsequent iterations.
func attestationDataMatchesChain(state *types.State, data *types.AttestationData) bool {
	if data.Source.Root == types.ZeroRoot || data.Target.Root == types.ZeroRoot {
		return false
	}
	histLen := uint64(len(state.HistoricalBlockHashes))
	if data.Source.Slot >= histLen || data.Target.Slot >= histLen {
		return false
	}
	if !bytes.Equal(data.Source.Root[:], state.HistoricalBlockHashes[data.Source.Slot]) {
		return false
	}
	if !bytes.Equal(data.Target.Root[:], state.HistoricalBlockHashes[data.Target.Slot]) {
		return false
	}
	return true
}

func compareRoots(a, b [32]byte) int {
	for i := range 32 {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
