package node

import (
	"context"
	"sort"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// AggregationDispatch carries one slot's aggregation work from the tick
// thread to the worker goroutine. The snapshot is taken synchronously on
// the tick thread (cheap, milliseconds); the prove and publish steps run
// on the worker.
type AggregationDispatch struct {
	snapshot *AggregationSnapshot
	slot     uint64
}

// runAggregationWorker drains AggregationDispatchCh and runs the prove +
// publish + apply phases off the tick loop. Mirrors ethlambda's
// tokio::spawn_blocking pattern (aggregation.rs:395-462) and zeam's worker
// threads — the consensus tick never blocks on the FFI.
//
// One worker; AggregationDispatchCh is buffered to 1. If a new dispatch
// arrives while the worker is mid-prove the tick-thread send drops it via
// the select default branch and increments lean_aggregation_dispatch_dropped_total.
// Drops are spec-permissible (aggregation is best-effort per slot).
func (e *Engine) runAggregationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch := <-e.AggregationDispatchCh:
			workerStart := time.Now()
			aggs, mut := aggregateFromSnapshot(dispatch.snapshot, e.Store.PubKeyCache)
			applyAggregationMutations(e.Store, mut)
			for _, agg := range aggs {
				if e.P2P != nil {
					e.P2P.PublishAggregatedAttestation(context.Background(), agg)
				}
			}
			ObserveAggregationWorkerTotalTime(time.Since(workerStart).Seconds())
			logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v",
				dispatch.slot, len(aggs), time.Since(workerStart))
		}
	}
}

// AggregationSnapshot captures all store reads aggregation needs in one pass,
// taken synchronously by snapshotAggregationInputs so the prove phase
// (aggregateFromSnapshot) can run as a pure function of (snapshot, pubkey
// cache) without holding a *ConsensusStore reference. Mirrors the structural
// split ethlambda uses (snapshot_aggregation_inputs → aggregate_job worker
// → publish) and is the prerequisite for off-tick async dispatch.
type AggregationSnapshot struct {
	headState    *types.State
	attSigs      map[[32]byte]*AttestationDataEntry
	newEntries   map[[32]byte]*PayloadEntry
	knownEntries map[[32]byte]*PayloadEntry
	targetStates map[[32]byte]*types.State // pre-resolved by attData.Target.Root
}

// AggregationMutations is the batched store change the prove phase wants to
// apply when it returns. applyAggregationMutations performs the two writes
// (KnownPayloads.PushBatch + AttestationSignatures.Delete) as a single unit.
type AggregationMutations struct {
	PayloadEntries []PayloadKV
	KeysToDelete   []AttestationDeleteKey
}

// snapshotAggregationInputs reads all store state aggregation needs into a
// consistent snapshot. Returns nil when there is nothing to aggregate.
func snapshotAggregationInputs(s *ConsensusStore) *AggregationSnapshot {
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
		newEntries:   make(map[[32]byte]*PayloadEntry),
		knownEntries: make(map[[32]byte]*PayloadEntry),
		targetStates: make(map[[32]byte]*types.State),
	}

	// Collect data roots that have either gossip sigs or new payloads and
	// copy the matching new/known payload entry refs into the snapshot.
	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr, entry := range s.NewPayloads.data {
		dataRoots[dr] = true
		snap.newEntries[dr] = entry
	}
	for dr := range dataRoots {
		if entry := s.KnownPayloads.data[dr]; entry != nil {
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
func aggregateFromSnapshot(snap *AggregationSnapshot, cache *xmss.PubKeyCache) ([]*types.SignedAggregatedAttestation, *AggregationMutations) {
	var newAggregates []*types.SignedAggregatedAttestation
	mut := &AggregationMutations{}

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

			ObserveAggregationPrepTime(time.Since(prepStart).Seconds())

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

			if AggregateMetricsFunc != nil {
				AggregateMetricsFunc(aggDuration.Seconds(), len(allIDs))
			}

			postStart := time.Now()
			newAggregates = append(newAggregates, &types.SignedAggregatedAttestation{
				Data:  attData,
				Proof: proof,
			})

			mut.PayloadEntries = append(mut.PayloadEntries, PayloadKV{
				DataRoot: dataRoot,
				Data:     attData,
				Proof:    proof,
			})

			if gossipEntry != nil {
				for _, sig := range gossipEntry.Signatures {
					mut.KeysToDelete = append(mut.KeysToDelete, AttestationDeleteKey{
						ValidatorID: sig.ValidatorID,
						DataRoot:    dataRoot,
					})
				}
			}
			ObserveAggregationPostTime(time.Since(postStart).Seconds())
		}()
	}

	return newAggregates, mut
}

// applyAggregationMutations applies the prove phase's batched store changes.
func applyAggregationMutations(s *ConsensusStore, m *AggregationMutations) {
	commitStart := time.Now()
	s.KnownPayloads.PushBatch(m.PayloadEntries)
	s.AttestationSignatures.Delete(m.KeysToDelete)
	ObserveAggregationCommitTime(time.Since(commitStart).Seconds())
}

// selectChildProofs greedily selects existing proofs from a payload entry,
// adding them as children and tracking covered validators.
func selectChildProofs(
	entry *PayloadEntry,
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
