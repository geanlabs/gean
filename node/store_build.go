package node

import (
	"fmt"
	"sort"

	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// ProduceBlockWithSignatures builds a block using per-AttestationData fixed-point selection.
// Returns the block and per-attestation signature proofs.
// Spec: lean_spec/subspecs/containers/state/state.py build_block
// Cross-ref: zeam getProposalAttestationsUnlocked, ethlambda build_block
func ProduceBlockWithSignatures(
	s *ConsensusStore,
	slot, validatorIndex uint64,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil {
		return nil, nil, &StoreError{ErrMissingParentState,
			fmt.Sprintf("head state missing for slot %d", slot)}
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		return nil, nil, errNotProposer(validatorIndex, slot)
	}

	knownEntries := s.KnownPayloads.Entries()
	knownBlockRoots := s.getBlockRoots()

	return buildBlock(headState, slot, validatorIndex, headRoot, knownBlockRoots, knownEntries)
}

// buildBlock builds a valid block using per-AttestationData fixed-point selection
// with greedy proof coverage and MAX_ATTESTATIONS_DATA cap.
//
// Algorithm (per spec build_block):
// 1. Sort payloads by target.slot for deterministic order
// 2. For each AttestationData whose source == current_justified:
//    a. Skip if head not in known_block_roots
//    b. Skip if already processed
//    c. Greedy proof selection: pick proofs maximizing new validator coverage
// 3. Trial STF — if justified advances, update source and continue
// 4. Enforce MAX_ATTESTATIONS_DATA cap
func buildBlock(
	headState *types.State,
	slot, proposerIndex uint64,
	parentRoot [32]byte,
	knownBlockRoots map[[32]byte]bool,
	payloads map[[32]byte]*PayloadEntry,
) (*types.Block, []*types.AggregatedSignatureProof, error) {
	var attestations []*types.AggregatedAttestation
	var signatures []*types.AggregatedSignatureProof

	if len(payloads) > 0 {
		// Genesis edge case: derive justified checkpoint matching process_block_header.
		// Spec: state.py build_block "if self.latest_block_header.slot == Slot(0)"
		var currentJustified *types.Checkpoint
		if headState.LatestBlockHeader.Slot == 0 {
			currentJustified = &types.Checkpoint{
				Root: parentRoot,
				Slot: headState.LatestJustified.Slot,
			}
		} else {
			currentJustified = headState.LatestJustified
		}

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
					continue
				}
				if item.entry.Data.Source.Root != currentJustified.Root ||
					item.entry.Data.Source.Slot != currentJustified.Slot {
					continue
				}

				processedAttData[item.dataRoot] = true
				foundEntries = true

				// Select best proof for this AttestationData (max validator coverage).
				// Phase 7 will add recursive compaction for multi-proof merging.
				selectBestProof(item.entry, &attestations, &signatures)
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

	return finalBlock, signatures, nil
}

// selectBestProof picks the single proof with maximum validator coverage from entry.
// Pre-Phase-7: selects one proof per AttestationData to avoid duplicate entries
// that on_block would reject. Phase 7 will add recursive children aggregation
// to merge multiple proofs per data into one.
// Matches spec select_greedily but limited to one proof until compact step exists.
func selectBestProof(
	entry *PayloadEntry,
	attestations *[]*types.AggregatedAttestation,
	signatures *[]*types.AggregatedSignatureProof,
) {
	if len(entry.Proofs) == 0 {
		return
	}

	bestIdx := -1
	bestCount := 0

	for i, proof := range entry.Proofs {
		count := 0
		bitsLen := types.BitlistLen(proof.Participants)
		for vid := uint64(0); vid < bitsLen; vid++ {
			if types.BitlistGet(proof.Participants, vid) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestIdx = i
		}
	}

	if bestIdx < 0 || bestCount == 0 {
		return
	}

	proof := entry.Proofs[bestIdx]
	*attestations = append(*attestations, &types.AggregatedAttestation{
		AggregationBits: proof.Participants,
		Data:            entry.Data,
	})
	*signatures = append(*signatures, proof)
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
