package node

import (
	"sort"
	"time"

	"github.com/geanlabs/gean/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// AggregateCommitteeSignatures collects gossip signatures and aggregates them
// using real XMSS ZK aggregation via xmss.AggregateSignatures.
func AggregateCommitteeSignatures(s *ConsensusStore) []*types.SignedAggregatedAttestation {
	if s.GossipSignatures.Len() == 0 {
		return nil
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var keysToDelete []GossipDeleteKey
	var payloadEntries []PayloadKV

	for dataRoot, entry := range s.GossipSignatures {
		if len(entry.Signatures) == 0 {
			continue
		}

		// Get target state for pubkey lookup.
		targetState := s.GetState(entry.Data.Target.Root)
		if targetState == nil {
			logger.Warn(logger.Signature, "aggregate: missing target state for %x", entry.Data.Target.Root)
			continue
		}

		// Sort signatures by validator ID for deterministic aggregation ordering.
		// Verification side uses BitlistIndices which returns ascending order,
		// so aggregation must match.
		sortedSigs := make([]GossipSignatureEntry, len(entry.Signatures))
		copy(sortedSigs, entry.Signatures)
		sort.Slice(sortedSigs, func(i, j int) bool {
			return sortedSigs[i].ValidatorID < sortedSigs[j].ValidatorID
		})

		// Collect pubkeys and signatures as opaque C handles.
		var pubkeys []xmss.CPubKey
		var sigs []xmss.CSig
		var ids []uint64
		var cleanupSigs []xmss.CSig // for fallback-parsed sigs only

		valid := true
		for _, sigEntry := range sortedSigs {
			if sigEntry.ValidatorID >= uint64(len(targetState.Validators)) {
				logger.Error(logger.Signature, "aggregate: validator %d out of range", sigEntry.ValidatorID)
				valid = false
				break
			}

			// Use stored C handle if available.
			// If no handle, parse from SSZ bytes (fallback for P2P proposer attestations).
			sigHandle := sigEntry.SigHandle
			if sigHandle == nil {
				parsed, err := xmss.ParseSignature(sigEntry.Signature[:])
				if err != nil {
					logger.Warn(logger.Signature, "aggregate: parse sig fallback for validator %d: %v", sigEntry.ValidatorID, err)
					valid = false
					break
				}
				cleanupSigs = append(cleanupSigs, parsed)
				sigHandle = parsed
			}

			// Get cached pubkey handle (parsed once, reused across aggregation cycles).
			pk, err := s.PubKeyCache.Get(targetState.Validators[sigEntry.ValidatorID].Pubkey)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: parse pubkey %d: %v", sigEntry.ValidatorID, err)
				valid = false
				break
			}

			pubkeys = append(pubkeys, pk)
			sigs = append(sigs, sigHandle)
			ids = append(ids, sigEntry.ValidatorID)
		}

		// Free only fallback-parsed sig handles. Pubkey handles are owned by the cache.
		defer func() {
			for _, sig := range cleanupSigs {
				xmss.FreeSignature(sig)
			}
		}()

		if !valid || len(ids) == 0 {
			continue
		}

		// Aggregate via real XMSS ZK proof.
		slot := uint32(entry.Data.Slot)
		aggStart := time.Now()
		proofBytes, err := xmss.AggregateSignatures(pubkeys, sigs, dataRoot, slot)
		aggDuration := time.Since(aggStart)
		if err != nil {
			logger.Error(logger.Signature, "aggregate: failed slot=%d sigs=%d validators=%v duration=%v: %v",
				slot, len(sigs), ids, aggDuration, err)
			continue
		}
		logger.Info(logger.Signature, "aggregate: slot=%d sigs=%d validators=%v proof=%d bytes duration=%v",
			slot, len(sigs), ids, len(proofBytes), aggDuration)

		// Metrics — imported from engine package via function references to avoid circular deps.
		if AggregateMetricsFunc != nil {
			AggregateMetricsFunc(aggDuration.Seconds(), len(ids))
		}

		participants := aggregationBitsFromValidatorIndices(ids)
		proof := &types.AggregatedSignatureProof{
			Participants: participants,
			ProofData:    proofBytes,
		}

		newAggregates = append(newAggregates, &types.SignedAggregatedAttestation{
			Data:  entry.Data,
			Proof: proof,
		})

		payloadEntries = append(payloadEntries, PayloadKV{
			DataRoot: dataRoot,
			Data:     entry.Data,
			Proof:    proof,
		})

		for _, id := range ids {
			keysToDelete = append(keysToDelete, GossipDeleteKey{
				ValidatorID: id,
				DataRoot:    dataRoot,
			})
		}
	}

	// Insert into known (immediately usable for block building and fork choice).
	
	s.KnownPayloads.PushBatch(payloadEntries)

	// Delete aggregated signatures from gossip store.
	s.GossipSignatures.Delete(keysToDelete)

	return newAggregates
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
