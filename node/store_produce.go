package node

import (
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/types"
)

// ProduceAttestationData creates attestation data for the given slot.
// Uses the head state's latest_justified as source per leanSpec and ethlambda.
// Cross-ref: ethlambda store.rs:743 produce_attestation_data, leanSpec store.py:1283
func ProduceAttestationData(s *ConsensusStore, slot uint64) *types.AttestationData {
	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil {
		return nil
	}

	// Source from store's justified (stable, converges via tiebreak).
	// At genesis substitute the real genesis block root for the zero hash.
	source := s.LatestJustified()
	if headState.LatestBlockHeader.Slot == 0 {
		source = &types.Checkpoint{
			Root: headRoot,
			Slot: source.Slot,
		}
	}

	headHeader := s.GetBlockHeader(headRoot)
	if headHeader == nil {
		return nil
	}
	headCheckpoint := &types.Checkpoint{
		Root: headRoot,
		Slot: headHeader.Slot,
	}

	target := GetAttestationTarget(s)

	logger.Info(logger.Chain, "ProduceAttestation: slot=%d head=0x%x source=0x%x/%d target=0x%x/%d",
		slot, headRoot, source.Root, source.Slot, target.Root, target.Slot)

	return &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: target,
		Source: source,
	}
}

// GetAttestationTarget computes the target checkpoint for attestations.
func GetAttestationTarget(s *ConsensusStore) *types.Checkpoint {
	targetRoot := s.Head()
	targetHeader := s.GetBlockHeader(targetRoot)
	if targetHeader == nil {
		return &types.Checkpoint{}
	}

	safeTargetHeader := s.GetBlockHeader(s.SafeTarget())
	safeTargetSlot := uint64(0)
	if safeTargetHeader != nil {
		safeTargetSlot = safeTargetHeader.Slot
	}

	// Walk back toward safe target (up to JUSTIFICATION_LOOKBACK_SLOTS steps).

	for i := uint64(0); i < types.JustificationLookbackSlots; i++ {
		if targetHeader.Slot > safeTargetSlot {
			targetRoot = targetHeader.ParentRoot
			parent := s.GetBlockHeader(targetRoot)
			if parent == nil {
				break
			}
			targetHeader = parent
		} else {
			break
		}
	}

	finalizedSlot := s.LatestFinalized().Slot

	// Walk back until justifiable slot.

	for targetHeader.Slot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetHeader.Slot, finalizedSlot) {
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}

	// Clamp to latest_justified if walked behind.

	latestJustified := s.LatestJustified()
	if targetHeader.Slot < latestJustified.Slot {
		return latestJustified
	}

	return &types.Checkpoint{
		Root: targetRoot,
		Slot: targetHeader.Slot,
	}
}
