package aggregation

import (
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func aggregateFromSnapshot(snap *Snapshot, cache *xmss.PubKeyCache) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey) {
	if snap == nil || cache == nil {
		return nil, nil, nil
	}

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

			attData := attestationDataForRoot(snap, dataRoot)
			if attData == nil {
				return
			}

			targetState := snap.targetStates[attData.Target.Root]
			if targetState == nil {
				return
			}

			covered := make(map[uint64]bool)
			selectChildProofs(newEntry, targetState, childProofsBuf, covered, cache)
			selectChildProofs(knownEntry, targetState, childProofsBuf, covered, cache)

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

			if len(*rawIDsBuf)+len(*childProofsBuf) < 2 {
				return
			}

			dataRootHash, slot, err := aggregationMessage(attData)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: prepare message failed slot=%d: %v", attData.Slot, err)
				return
			}

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

			proof := &types.SingleMessageAggregate{
				Participants: types.BitlistFromIndices(allIDs),
				Proof:        proofBytes,
			}

			logger.Info(logger.Signature, "aggregate: slot=%d raw=%d children=%d total=%d proof=%d bytes duration=%v",
				slot, len(*rawIDsBuf), len(*childProofsBuf), len(allIDs), len(proofBytes), aggDuration)

			metrics.ObservePqSigAggBuildingTime(aggDuration.Seconds())
			metrics.ObserveCommitteeSignaturesAggregationTime(aggDuration.Seconds())
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

func aggregationMessage(attData *types.AttestationData) ([32]byte, uint32, error) {
	if attData == nil {
		return [32]byte{}, 0, fmt.Errorf("attestation data is nil")
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return [32]byte{}, 0, fmt.Errorf("attestation data root: %w", err)
	}
	slot := uint32(attData.Slot)
	if uint64(slot) != attData.Slot {
		return [32]byte{}, 0, fmt.Errorf("slot %d overflows uint32", attData.Slot)
	}
	return dataRoot, slot, nil
}
