package node

import (
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/types"
)

// ProduceAttestationData creates attestation data for the given slot.
func ProduceAttestationData(s *ConsensusStore, slot uint64) *types.AttestationData {
	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil {
		return nil
	}

	// Derive source from head state's justified checkpoint.
	// At genesis the checkpoint root is zero; substitute the real genesis block root.

	var source *types.Checkpoint
	if headState.LatestBlockHeader.Slot == 0 {
		source = &types.Checkpoint{
			Root: headRoot,
			Slot: headState.LatestJustified.Slot,
		}
	} else {
		source = headState.LatestJustified
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
