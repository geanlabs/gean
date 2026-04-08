package node

import (
	"fmt"
	"sort"

	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// ProduceBlockWithSignatures builds a block using per-validator latest-vote selection.
// Returns the block and per-attestation signature proofs.
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

// buildBlock builds a valid block using per-validator latest-vote selection.
//
// For each validator, we pick their latest vote whose source matches the
// current justified checkpoint, then group validators by their vote's data
// root and emit one attestation per (data root, validator subset) pair.
//
// This bounds block size by the validator count: at most numValidators
// distinct attestations per fixed-point iteration. Multiple validators
// voting for the same target share a single AggregatedAttestation.
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

		// Track validators already included to avoid duplication across iterations.
		processedValidators := make(map[uint64]bool)

		for {
			// For the current justified source, find each validator's latest vote.
			// The result maps each validator to the payload entry containing their
			// latest matching vote (highest data.Slot).
			perValidator := selectLatestPerValidator(payloads, knownBlockRoots, currentJustified, processedValidators)
			if len(perValidator) == 0 {
				break
			}

			// Group validators by their selected payload entry. Multiple validators
			// pointing at the same entry will share AggregatedAttestations.
			groups := groupValidatorsByEntry(perValidator)

			added := 0
			for _, group := range groups {
				added += emitAttestationsForGroup(group.entry, group.validators, &attestations, &signatures, processedValidators)
			}
			if added == 0 {
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

// selectLatestPerValidator finds, for each validator, the payload entry that
// contains their latest vote whose source matches `currentJustified`.
//
// Validators in `excluded` are skipped (used to avoid re-selecting validators
// already included in earlier fixed-point iterations).
func selectLatestPerValidator(
	payloads map[[32]byte]*PayloadEntry,
	knownBlockRoots map[[32]byte]bool,
	currentJustified *types.Checkpoint,
	excluded map[uint64]bool,
) map[uint64]*PayloadEntry {
	perValidator := make(map[uint64]*PayloadEntry)
	for _, entry := range payloads {
		if !knownBlockRoots[entry.Data.Head.Root] {
			continue
		}
		if entry.Data.Source.Root != currentJustified.Root ||
			entry.Data.Source.Slot != currentJustified.Slot {
			continue
		}
		for _, proof := range entry.Proofs {
			for _, vid := range types.BitlistIndices(proof.Participants) {
				if excluded[vid] {
					continue
				}
				existing, ok := perValidator[vid]
				if !ok || entry.Data.Slot > existing.Data.Slot {
					perValidator[vid] = entry
				}
			}
		}
	}
	return perValidator
}

// validatorGroup holds a payload entry and the validators selected from it.
type validatorGroup struct {
	entry      *PayloadEntry
	validators []uint64
}

// groupValidatorsByEntry inverts perValidator into groups keyed by entry,
// returning a deterministically-sorted slice. Multiple validators pointing
// at the same entry are batched so we can pick proofs that cover them all.
func groupValidatorsByEntry(perValidator map[uint64]*PayloadEntry) []validatorGroup {
	byEntry := make(map[*PayloadEntry][]uint64)
	for vid, entry := range perValidator {
		byEntry[entry] = append(byEntry[entry], vid)
	}
	groups := make([]validatorGroup, 0, len(byEntry))
	for entry, vids := range byEntry {
		sort.Slice(vids, func(i, j int) bool { return vids[i] < vids[j] })
		groups = append(groups, validatorGroup{entry: entry, validators: vids})
	}
	// Deterministic order: by target slot then by data root.
	sort.Slice(groups, func(i, j int) bool {
		ei, ej := groups[i].entry, groups[j].entry
		if ei.Data.Target.Slot != ej.Data.Target.Slot {
			return ei.Data.Target.Slot < ej.Data.Target.Slot
		}
		ri, _ := ei.Data.HashTreeRoot()
		rj, _ := ej.Data.HashTreeRoot()
		return compareRoots(ri, rj) < 0
	})
	return groups
}

// emitAttestationsForGroup picks the smallest set of proofs from `entry` that
// covers all validators in `wanted`, appending one AggregatedAttestation per
// chosen proof. Returns the number of attestations emitted.
func emitAttestationsForGroup(
	entry *PayloadEntry,
	wanted []uint64,
	attestations *[]*types.AggregatedAttestation,
	signatures *[]*types.AggregatedSignatureProof,
	processedValidators map[uint64]bool,
) int {
	needed := make(map[uint64]bool, len(wanted))
	for _, vid := range wanted {
		needed[vid] = true
	}

	emitted := 0
	for len(needed) > 0 {
		bestIdx := -1
		bestCount := 0
		for i, proof := range entry.Proofs {
			count := 0
			for _, vid := range types.BitlistIndices(proof.Participants) {
				if needed[vid] {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestIdx = i
			}
		}
		if bestIdx < 0 || bestCount == 0 {
			break
		}

		proof := entry.Proofs[bestIdx]
		*attestations = append(*attestations, &types.AggregatedAttestation{
			AggregationBits: proof.Participants,
			Data:            entry.Data,
		})
		*signatures = append(*signatures, proof)
		emitted++

		for _, vid := range types.BitlistIndices(proof.Participants) {
			delete(needed, vid)
			processedValidators[vid] = true
		}
	}
	return emitted
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
