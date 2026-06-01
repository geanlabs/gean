package aggregation

import (
	"context"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// Publisher is the aggregation worker's outbound gossip boundary.
type Publisher interface {
	PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error
}

// Dispatch carries one slot's aggregation work from the tick thread to the
// worker goroutine. The snapshot is taken synchronously on the tick thread;
// proving and publishing run on the worker.
type Dispatch struct {
	Snapshot *AggregationSnapshot
	Slot     uint64
}

// RunWorker drains aggregation dispatches and runs the prove, publish, and
// apply phases off the tick loop. Drops are handled by the sender.
func RunWorker(
	ctx context.Context,
	dispatches <-chan Dispatch,
	store *store.ConsensusStore,
	cache *xmss.PubKeyCache,
	publisher Publisher,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch := <-dispatches:
			workerStart := time.Now()
			aggs, payloads, deletes := aggregateFromSnapshot(dispatch.Snapshot, cache)
			applyAggregationMutations(store, payloads, deletes)
			for _, agg := range aggs {
				if publisher != nil {
					_ = publisher.PublishAggregatedAttestation(context.Background(), agg)
				}
			}
			metrics.ObserveAggregationWorkerTotalTime(time.Since(workerStart).Seconds())
			logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v",
				dispatch.Slot, len(aggs), time.Since(workerStart))
		}
	}
}

// AggregationSnapshot captures all store reads aggregation needs in one pass,
// taken synchronously by SnapshotInputs so the prove phase
// (aggregateFromSnapshot) can run as a pure function of (snapshot, pubkey
// cache) without holding a *store.ConsensusStore reference. This snapshot →
// worker → publish split is the prerequisite for off-tick async dispatch.
type AggregationSnapshot struct {
	headState    *types.State
	attSigs      map[[32]byte]*store.AttestationDataEntry
	newEntries   map[[32]byte]*store.PayloadEntry
	knownEntries map[[32]byte]*store.PayloadEntry
	targetStates map[[32]byte]*types.State // pre-resolved by attData.Target.Root
}

// SnapshotInputs reads all store state aggregation needs into a
// consistent snapshot. Returns nil when there is nothing to aggregate.
func SnapshotInputs(s *store.ConsensusStore) *AggregationSnapshot {
	if s.AttestationSignatures.Len() == 0 && s.NewPayloads.Len() == 0 {
		return nil
	}
	headState := s.GetState(s.Head())
	if headState == nil {
		return nil
	}

	snap := &AggregationSnapshot{
		headState:    headState,
		attSigs:      s.AttestationSignatures.Snapshot(),
		newEntries:   make(map[[32]byte]*store.PayloadEntry),
		knownEntries: make(map[[32]byte]*store.PayloadEntry),
		targetStates: make(map[[32]byte]*types.State),
	}

	// Collect data roots that have either gossip sigs or new payloads and
	// copy the matching new/known payload entry refs into the snapshot.
	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr, entry := range s.NewPayloads.Entries() {
		dataRoots[dr] = true
		snap.newEntries[dr] = entry
	}
	for dr := range dataRoots {
		if entry := s.KnownPayloads.Entries()[dr]; entry != nil {
			snap.knownEntries[dr] = entry
		}
	}

	// Pre-resolve target states for each data root's attData so the prove
	// phase doesn't have to call back into the store.
	for dr := range dataRoots {
		var attData *types.AttestationData
		if e := snap.attSigs[dr]; e != nil {
			attData = e.Data
		} else if e := snap.newEntries[dr]; e != nil {
			attData = e.Data
		} else if e := snap.knownEntries[dr]; e != nil {
			attData = e.Data
		}
		if attData == nil {
			continue
		}
		if _, ok := snap.targetStates[attData.Target.Root]; !ok {
			if state := s.GetState(attData.Target.Root); state != nil {
				snap.targetStates[attData.Target.Root] = state
			}
		}
	}

	return snap
}

// aggregateFromSnapshot runs the per-data-root prep + FFI + post phases.
// Pure function of (snapshot, pubkey cache) — performs no store reads — so
// it can later run on a worker goroutine without holding a store reference.
func aggregateFromSnapshot(snap *AggregationSnapshot, cache *xmss.PubKeyCache) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey) {
	var newAggregates []*types.SignedAggregatedAttestation
	var payloadEntries []store.PayloadKV
	var keysToDelete []store.AttestationDeleteKey

	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr := range snap.newEntries {
		dataRoots[dr] = true
	}

	for dataRoot := range dataRoots {
		// Anonymous func per iteration so pooled scratch slices (and the
		// defer xmss.FreeSignature inside the sig loop) release per data
		// root rather than accumulating until aggregateFromSnapshot returns.
		func() {
			childProofsBuf := getChildProofsBuf()
			defer putChildProofsBuf(childProofsBuf)
			rawPubkeysBuf := getRawPubkeysBuf()
			defer putRawPubkeysBuf(rawPubkeysBuf)
			rawSigsBuf := getRawSigsBuf()
			defer putRawSigsBuf(rawSigsBuf)
			rawIDsBuf := getRawIDsBuf()
			defer putRawIDsBuf(rawIDsBuf)

			prepStart := time.Now()
			gossipEntry := snap.attSigs[dataRoot]
			newEntry := snap.newEntries[dataRoot]
			knownEntry := snap.knownEntries[dataRoot]

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
				return
			}

			targetState := snap.targetStates[attData.Target.Root]
			if targetState == nil {
				return
			}

			// Phase 1: Select — greedy pick existing child proofs.
			covered := make(map[uint64]bool)
			selectChildProofs(newEntry, targetState, childProofsBuf, covered, cache)
			selectChildProofs(knownEntry, targetState, childProofsBuf, covered, cache)

			// Phase 2: Fill — collect raw gossip signatures for uncovered validators.
			if gossipEntry != nil && len(gossipEntry.Signatures) > 0 {
				sortedSigs := make([]store.AttestationSignatureEntry, len(gossipEntry.Signatures))
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

					pk, err := cache.Get(targetState.Validators[sigEntry.ValidatorID].AttestationPubkey)
					if err != nil {
						continue
					}

					*rawPubkeysBuf = append(*rawPubkeysBuf, pk)
					*rawSigsBuf = append(*rawSigsBuf, sigHandle)
					*rawIDsBuf = append(*rawIDsBuf, sigEntry.ValidatorID)
				}
			}

			// Prover requires at least 2 total inputs.
			if len(*rawIDsBuf)+len(*childProofsBuf) < 2 {
				return
			}

			// Phase 3: Aggregate — produce recursive proof.
			dataRootHash, _ := attData.HashTreeRoot()
			slot := uint32(attData.Slot)

			metrics.ObserveAggregationPrepTime(time.Since(prepStart).Seconds())

			aggStart := time.Now()
			proofBytes, err := xmss.AggregateWithChildren(*rawPubkeysBuf, *rawSigsBuf, *childProofsBuf, dataRootHash, slot)
			aggDuration := time.Since(aggStart)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: failed slot=%d raw=%d children=%d duration=%v: %v",
					slot, len(*rawIDsBuf), len(*childProofsBuf), aggDuration, err)
				return
			}

			allIDs := make([]uint64, 0, len(*rawIDsBuf)+len(covered))
			allIDs = append(allIDs, (*rawIDsBuf)...)
			for vid := range covered {
				allIDs = append(allIDs, vid)
			}

			participants := AggregationBitsFromIndices(allIDs)
			proof := &types.AggregatedSignatureProof{
				Participants: participants,
				ProofData:    proofBytes,
			}

			logger.Info(logger.Signature, "aggregate: slot=%d raw=%d children=%d total=%d proof=%d bytes duration=%v",
				slot, len(*rawIDsBuf), len(*childProofsBuf), len(allIDs), len(proofBytes), aggDuration)

			metrics.ObservePqSigAggBuildingTime(aggDuration.Seconds())
			metrics.IncPqSigAggregatedTotal()
			metrics.IncPqSigAttestationsInAggregated(len(allIDs))

			newAggregates = append(newAggregates, &types.SignedAggregatedAttestation{
				Data:  attData,
				Proof: proof,
			})

			payloadEntries = append(payloadEntries, store.PayloadKV{
				DataRoot: dataRoot,
				Data:     attData,
				Proof:    proof,
			})

			if gossipEntry != nil {
				for _, sig := range gossipEntry.Signatures {
					keysToDelete = append(keysToDelete, store.AttestationDeleteKey{
						ValidatorID: sig.ValidatorID,
						DataRoot:    dataRoot,
					})
				}
			}
		}()
	}

	return newAggregates, payloadEntries, keysToDelete
}

// applyAggregationMutations applies the prove phase's batched store changes.
func applyAggregationMutations(s *store.ConsensusStore, payloads []store.PayloadKV, deletes []store.AttestationDeleteKey) {
	s.KnownPayloads.PushBatch(payloads)
	s.AttestationSignatures.Delete(deletes)
}

// selectChildProofs greedily selects existing proofs from a payload entry,
// adding them as children and tracking covered validators.
func selectChildProofs(
	entry *store.PayloadEntry,
	state *types.State,
	children *[]xmss.ChildProof,
	covered map[uint64]bool,
	cache *xmss.PubKeyCache,
) {
	if entry == nil || len(entry.Proofs) == 0 {
		return
	}

	for _, proof := range entry.Proofs {
		newCoverage := 0
		bitsLen := types.BitlistLen(proof.Participants)
		for vid := range bitsLen {
			if types.BitlistGet(proof.Participants, vid) && !covered[vid] {
				newCoverage++
			}
		}
		if newCoverage == 0 {
			continue
		}

		var pubkeys []xmss.CPubKey
		for vid := range bitsLen {
			if types.BitlistGet(proof.Participants, vid) {
				if vid < uint64(len(state.Validators)) {
					pk, err := cache.Get(state.Validators[vid].AttestationPubkey)
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
