package node

import (
	"sort"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// AggregationSnapshot holds inputs for background aggregation.
// Created on the main goroutine, exclusively owned by the aggregation goroutine.
type AggregationSnapshot struct {
	GossipSigs    AttestationSignatureMap
	NewPayloads   map[[32]byte]*PayloadEntry
	KnownPayloads map[[32]byte]*PayloadEntry
	HeadState     *types.State
	PubKeyCache   *xmss.PubKeyCache
}

// AggregationResult holds output from background aggregation.
type AggregationResult struct {
	Aggregates     []*types.SignedAggregatedAttestation
	PayloadEntries []PayloadKV
	UnconsumedSigs AttestationSignatureMap
}

// TakeAggregationSnapshot captures inputs for async aggregation.
// Swaps AttestationSignatures with a fresh empty map (caller keeps fresh one,
// snapshot owns the old one). Deep-copies payload maps to avoid races.
func TakeAggregationSnapshot(s *ConsensusStore) *AggregationSnapshot {
	gossipSnap := s.AttestationSignatures
	s.AttestationSignatures = make(AttestationSignatureMap)

	newSnap := make(map[[32]byte]*PayloadEntry, len(s.NewPayloads.data))
	for k, v := range s.NewPayloads.data {
		proofsCopy := make([]*types.AggregatedSignatureProof, len(v.Proofs))
		copy(proofsCopy, v.Proofs)
		newSnap[k] = &PayloadEntry{Data: v.Data, Proofs: proofsCopy}
	}

	knownSnap := make(map[[32]byte]*PayloadEntry, len(s.KnownPayloads.data))
	for k, v := range s.KnownPayloads.data {
		proofsCopy := make([]*types.AggregatedSignatureProof, len(v.Proofs))
		copy(proofsCopy, v.Proofs)
		knownSnap[k] = &PayloadEntry{Data: v.Data, Proofs: proofsCopy}
	}

	return &AggregationSnapshot{
		GossipSigs:    gossipSnap,
		NewPayloads:   newSnap,
		KnownPayloads: knownSnap,
		HeadState:     s.GetState(s.Head()),
		PubKeyCache:   s.PubKeyCache,
	}
}

// AggregateCommitteeSignatures runs synchronously on the store (testing/fallback).
func AggregateCommitteeSignatures(s *ConsensusStore) []*types.SignedAggregatedAttestation {
	snap := TakeAggregationSnapshot(s)
	result := AggregateFromSnapshot(snap)
	s.KnownPayloads.PushBatch(result.PayloadEntries)
	// Return unconsumed gossip sigs to the store so they accumulate
	// until there are enough to aggregate (minimum 2 inputs).
	for dr, entry := range result.UnconsumedSigs {
		s.AttestationSignatures[dr] = entry
	}
	return result.Aggregates
}

// AggregateFromSnapshot implements the three-phase Select/Fill/Aggregate
// algorithm from leanSpec store.py aggregate(). Safe to call from a goroutine
// since it operates only on the snapshot data.
//
// Cross-ref: leanSpec store.py:936-1071, zeam forkchoice.zig aggregateUnlocked
func AggregateFromSnapshot(snap *AggregationSnapshot) AggregationResult {
	if len(snap.GossipSigs) == 0 && len(snap.NewPayloads) == 0 {
		return AggregationResult{}
	}

	headState := snap.HeadState
	if headState == nil {
		return AggregationResult{}
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var payloadEntries []PayloadKV
	consumedDataRoots := make(map[[32]byte]bool)

	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.GossipSigs {
		dataRoots[dr] = true
	}
	for dr := range snap.NewPayloads {
		dataRoots[dr] = true
	}

	for dataRoot := range dataRoots {
		gossipEntry := snap.GossipSigs[dataRoot]
		newEntry := snap.NewPayloads[dataRoot]
		knownEntry := snap.KnownPayloads[dataRoot]

		// Need attestation data from any available source.
		var attData *types.AttestationData
		if gossipEntry != nil {
			attData = gossipEntry.Data
		} else if newEntry != nil {
			attData = newEntry.Data
		} else if knownEntry != nil {
			attData = knownEntry.Data
		}
		if attData == nil {
			continue
		}

		targetState := snap.HeadState
		if attData.Target.Root != [32]byte{} {
			// Use target state for validator lookup if available.
			// In snapshot mode we don't have arbitrary state access, so fall back
			// to head state which has the same validator set in devnet.
		}

		// Phase 1: Select — greedy pick existing child proofs.
		// New payloads before known (per spec priority order).
		var childProofs []xmss.ChildProof
		covered := make(map[uint64]bool)

		selectChildProofsFromMap(newEntry, targetState, &childProofs, covered, snap.PubKeyCache)
		selectChildProofsFromMap(knownEntry, targetState, &childProofs, covered, snap.PubKeyCache)

		// Phase 2: Fill — collect raw gossip signatures for uncovered validators.
		var rawPubkeys []xmss.CPubKey
		var rawSigs []xmss.CSig
		var rawIDs []uint64

		if gossipEntry != nil && len(gossipEntry.Signatures) > 0 {
			// Sort by validator ID for deterministic ordering.
			sortedSigs := make([]AttestationSignatureEntry, len(gossipEntry.Signatures))
			copy(sortedSigs, gossipEntry.Signatures)
			sort.Slice(sortedSigs, func(i, j int) bool {
				return sortedSigs[i].ValidatorID < sortedSigs[j].ValidatorID
			})

			for _, sigEntry := range sortedSigs {
				if covered[sigEntry.ValidatorID] {
					continue // Already covered by a child proof
				}
				if sigEntry.ValidatorID >= uint64(len(targetState.Validators)) {
					continue
				}

				sigHandle := sigEntry.SigHandle
				if sigHandle == nil {
					parsed, err := xmss.ParseSignature(sigEntry.Signature[:])
					if err != nil {
						continue
					}
					defer xmss.FreeSignature(parsed)
					sigHandle = parsed
				}

				pk, err := snap.PubKeyCache.Get(targetState.Validators[sigEntry.ValidatorID].AttestationPubkey)
				if err != nil {
					continue
				}

				rawPubkeys = append(rawPubkeys, pk)
				rawSigs = append(rawSigs, sigHandle)
				rawIDs = append(rawIDs, sigEntry.ValidatorID)
			}
		}

		// Prover requires at least 2 total inputs. A single raw sig with no
		// children causes an index-out-of-bounds panic in the sumcheck backend.
		totalInputs := len(rawIDs) + len(childProofs)
		if totalInputs < 2 {
			continue
		}

		// Phase 3: Aggregate — produce recursive proof.
		dataRootHash, _ := attData.HashTreeRoot()
		slot := uint32(attData.Slot)

		aggStart := time.Now()
		proofBytes, err := xmss.AggregateWithChildren(rawPubkeys, rawSigs, childProofs, dataRootHash, slot)
		aggDuration := time.Since(aggStart)
		if err != nil {
			logger.Error(logger.Signature, "aggregate: failed slot=%d raw=%d children=%d duration=%v: %v",
				slot, len(rawIDs), len(childProofs), aggDuration, err)
			continue
		}

		// Build merged participants bitlist.
		allIDs := make([]uint64, 0, len(rawIDs))
		allIDs = append(allIDs, rawIDs...)
		for vid := range covered {
			allIDs = append(allIDs, vid)
		}

		participants := aggregationBitsFromValidatorIndices(allIDs)
		proof := &types.AggregatedSignatureProof{
			Participants: participants,
			ProofData:    proofBytes,
		}

		logger.Info(logger.Signature, "aggregate: slot=%d raw=%d children=%d total=%d proof=%d bytes duration=%v",
			slot, len(rawIDs), len(childProofs), len(allIDs), len(proofBytes), aggDuration)

		if AggregateMetricsFunc != nil {
			AggregateMetricsFunc(aggDuration.Seconds(), len(allIDs))
		}

		newAggregates = append(newAggregates, &types.SignedAggregatedAttestation{
			Data:  attData,
			Proof: proof,
		})

		payloadEntries = append(payloadEntries, PayloadKV{
			DataRoot: dataRoot,
			Data:     attData,
			Proof:    proof,
		})

		consumedDataRoots[dataRoot] = true
	}

	// Free C handles only from CONSUMED gossip sigs.
	// Return unconsumed sigs to the result so they can be put back in the store.
	var unconsumedSigs AttestationSignatureMap
	for dr, entry := range snap.GossipSigs {
		if consumedDataRoots[dr] {
			for _, sig := range entry.Signatures {
				if sig.SigHandle != nil && FreeSignatureFunc != nil {
					FreeSignatureFunc(sig.SigHandle)
				}
			}
		} else {
			if unconsumedSigs == nil {
				unconsumedSigs = make(AttestationSignatureMap)
			}
			unconsumedSigs[dr] = entry
		}
	}

	return AggregationResult{
		UnconsumedSigs: unconsumedSigs,
		Aggregates:     newAggregates,
		PayloadEntries: payloadEntries,
	}
}

// selectChildProofsFromMap greedily selects existing proofs from a payload entry,
// adding them as children and tracking covered validators.
// Uses PubKeyCache directly (thread-safe via mutex).
func selectChildProofsFromMap(
	entry *PayloadEntry,
	state *types.State,
	children *[]xmss.ChildProof,
	covered map[uint64]bool,
	pkCache *xmss.PubKeyCache,
) {
	if entry == nil || len(entry.Proofs) == 0 {
		return
	}

	for _, proof := range entry.Proofs {
		// Check if this proof adds any new coverage.
		newCoverage := 0
		bitsLen := types.BitlistLen(proof.Participants)
		for vid := uint64(0); vid < bitsLen; vid++ {
			if types.BitlistGet(proof.Participants, vid) && !covered[vid] {
				newCoverage++
			}
		}
		if newCoverage == 0 {
			continue
		}

		// Collect pubkeys for this proof's participants.
		var pubkeys []xmss.CPubKey
		for vid := uint64(0); vid < bitsLen; vid++ {
			if types.BitlistGet(proof.Participants, vid) {
				if vid < uint64(len(state.Validators)) {
					pk, err := pkCache.Get(state.Validators[vid].AttestationPubkey)
					if err == nil {
						pubkeys = append(pubkeys, pk)
					}
				}
				covered[vid] = true
			}
		}

		*children = append(*children, xmss.ChildProof{
			Pubkeys:   pubkeys,
			ProofData: proof.ProofData,
		})
	}
}

// aggregationBitsFromValidatorIndices builds a bitlist from validator IDs.
func aggregationBitsFromValidatorIndices(ids []uint64) []byte {
	if len(ids) == 0 {
		return types.NewBitlistSSZ(0)
	}
	maxID := uint64(0)
	for _, id := range ids {
		if id > maxID {
			maxID = id
		}
	}
	bits := types.NewBitlistSSZ(maxID + 1)
	for _, id := range ids {
		types.BitlistSet(bits, id)
	}
	return bits
}
