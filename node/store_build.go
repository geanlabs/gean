package node

import (
	"fmt"
	"sort"

	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// ProduceBlockWithSignatures builds a block with greedy attestation selection.
// Returns the block and per-attestation signature proofs.
// Matches ethlambda store.rs produce_block_with_signatures (L747-786).
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

// buildBlock builds a valid block with greedy attestation selection.
// Matches ethlambda store.rs build_block (L975-1076).
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
		var currentJustified *types.Checkpoint
		if headState.LatestBlockHeader.Slot == 0 {
			currentJustified = &types.Checkpoint{
				Root: parentRoot,
				Slot: headState.LatestJustified.Slot,
			}
		} else {
			currentJustified = headState.LatestJustified
		}

		processedRoots := make(map[[32]byte]bool)

		// Sort by (target.slot, data_root) for deterministic processing order.
		type sortEntry struct {
			dataRoot [32]byte
			entry    *PayloadEntry
		}
		sorted := make([]sortEntry, 0, len(payloads))
		for dr, entry := range payloads {
			sorted = append(sorted, sortEntry{dataRoot: dr, entry: entry})
		}
		sort.Slice(sorted, func(i, j int) bool {
			si, sj := sorted[i], sorted[j]
			if si.entry.Data.Target.Slot != sj.entry.Data.Target.Slot {
				return si.entry.Data.Target.Slot < sj.entry.Data.Target.Slot
			}
			return compareRoots(si.dataRoot, sj.dataRoot) < 0
		})

		for {
			foundNew := false

			for _, se := range sorted {
				if processedRoots[se.dataRoot] {
					continue
				}
				if !knownBlockRoots[se.entry.Data.Head.Root] {
					continue
				}
				if se.entry.Data.Source.Root != currentJustified.Root ||
					se.entry.Data.Source.Slot != currentJustified.Slot {
					continue
				}

				processedRoots[se.dataRoot] = true
				foundNew = true

				extendProofsGreedily(se.entry.Proofs, &signatures, &attestations, se.entry.Data)
			}

			if !foundNew {
				break
			}

			// Check if justification advanced by trial state transition.
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
				// Continue: new checkpoint may unlock more attestation data.
			} else {
				break
			}
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

// extendProofsGreedily selects proofs maximizing new validator coverage.
// Matches ethlambda store.rs extend_proofs_greedily (L909-965).
func extendProofsGreedily(
	proofs []*types.AggregatedSignatureProof,
	selectedProofs *[]*types.AggregatedSignatureProof,
	attestations *[]*types.AggregatedAttestation,
	attData *types.AttestationData,
) {
	if len(proofs) == 0 {
		return
	}

	covered := make(map[uint64]bool)
	remaining := make(map[int]bool)
	for i := range proofs {
		remaining[i] = true
	}

	for len(remaining) > 0 {
		bestIdx := -1
		bestCount := 0

		for idx := range remaining {
			count := 0
			for _, vid := range types.BitlistIndices(proofs[idx].Participants) {
				if !covered[vid] {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestIdx = idx
			}
		}

		if bestIdx < 0 || bestCount == 0 {
			break
		}

		proof := proofs[bestIdx]
		*attestations = append(*attestations, &types.AggregatedAttestation{
			AggregationBits: proof.Participants,
			Data:            attData,
		})
		*selectedProofs = append(*selectedProofs, proof)

		for _, vid := range types.BitlistIndices(proof.Participants) {
			covered[vid] = true
		}
		delete(remaining, bestIdx)
	}
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
