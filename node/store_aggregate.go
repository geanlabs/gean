package node

import (
	"sort"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// AggregateCommitteeSignatures implements the three-phase Select/Fill/Aggregate
// algorithm from leanSpec store.py aggregate().
//
// For each AttestationData with new payloads or raw gossip signatures:
//  1. Select — greedily pick existing child proofs (new before known)
//  2. Fill — collect raw gossip signatures for uncovered validators
//  3. Aggregate — produce recursive proof with children + raw sigs
//
// Spec: lean_spec/subspecs/forkchoice/store.py aggregate
func AggregateCommitteeSignatures(s *ConsensusStore) []*types.SignedAggregatedAttestation {
	if s.AttestationSignatures.Len() == 0 && s.NewPayloads.Len() == 0 {
		return nil
	}

	headState := s.GetState(s.Head())
	if headState == nil {
		return nil
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var keysToDelete []AttestationDeleteKey
	var payloadEntries []PayloadKV

	// Snapshot attestation signatures to avoid holding the mutex during ZK proving.
	attSigs := s.AttestationSignatures.Snapshot()

	// Collect all data roots that have either gossip sigs or new payloads.
	dataRoots := make(map[[32]byte]bool)
	for dr := range attSigs {
		dataRoots[dr] = true
	}
	for dr := range s.NewPayloads.data {
		dataRoots[dr] = true
	}

	for dataRoot := range dataRoots {
		gossipEntry := attSigs[dataRoot]
		newEntry := s.NewPayloads.data[dataRoot]
		knownEntry := s.KnownPayloads.data[dataRoot]

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

		targetState := s.GetState(attData.Target.Root)
		if targetState == nil {
			continue
		}

		// Phase 1: Select — greedy pick existing child proofs.
		var childProofs []xmss.ChildProof
		covered := make(map[uint64]bool)

		selectChildProofs(newEntry, targetState, &childProofs, covered, s)
		selectChildProofs(knownEntry, targetState, &childProofs, covered, s)

		// Phase 2: Fill — collect raw gossip signatures for uncovered validators.
		var rawPubkeys []xmss.CPubKey
		var rawSigs []xmss.CSig
		var rawIDs []uint64

		if gossipEntry != nil && len(gossipEntry.Signatures) > 0 {
			sortedSigs := make([]AttestationSignatureEntry, len(gossipEntry.Signatures))
			copy(sortedSigs, gossipEntry.Signatures)
			sort.Slice(sortedSigs, func(i, j int) bool {
				return sortedSigs[i].ValidatorID < sortedSigs[j].ValidatorID
			})

			for _, sigEntry := range sortedSigs {
				if covered[sigEntry.ValidatorID] {
					continue
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

				pk, err := s.PubKeyCache.Get(targetState.Validators[sigEntry.ValidatorID].AttestationPubkey)
				if err != nil {
					continue
				}

				rawPubkeys = append(rawPubkeys, pk)
				rawSigs = append(rawSigs, sigHandle)
				rawIDs = append(rawIDs, sigEntry.ValidatorID)
			}
		}

		// Prover requires at least 2 total inputs.
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

		allIDs := make([]uint64, 0, len(rawIDs))
		allIDs = append(allIDs, rawIDs...)
		for vid := range covered {
			allIDs = append(allIDs, vid)
		}

		participants := AggregationBitsFromIndices(allIDs)
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

		if gossipEntry != nil {
			for _, sig := range gossipEntry.Signatures {
				keysToDelete = append(keysToDelete, AttestationDeleteKey{
					ValidatorID: sig.ValidatorID,
					DataRoot:    dataRoot,
				})
			}
		}
	}

	s.KnownPayloads.PushBatch(payloadEntries)
	s.AttestationSignatures.Delete(keysToDelete)

	return newAggregates
}

// selectChildProofs greedily selects existing proofs from a payload entry,
// adding them as children and tracking covered validators.
func selectChildProofs(
	entry *PayloadEntry,
	state *types.State,
	children *[]xmss.ChildProof,
	covered map[uint64]bool,
	s *ConsensusStore,
) {
	if entry == nil || len(entry.Proofs) == 0 {
		return
	}

	for _, proof := range entry.Proofs {
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

		var pubkeys []xmss.CPubKey
		for vid := uint64(0); vid < bitsLen; vid++ {
			if types.BitlistGet(proof.Participants, vid) {
				if vid < uint64(len(state.Validators)) {
					pk, err := s.PubKeyCache.Get(state.Validators[vid].AttestationPubkey)
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

// AggregationBitsFromIndices builds a bitlist from validator IDs.
func AggregationBitsFromIndices(ids []uint64) []byte {
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
