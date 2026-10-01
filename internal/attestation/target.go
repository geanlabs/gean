package attestation

import (
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func GetAttestationTarget(s *store.ConsensusStore) *types.Checkpoint {
	targetRoot := s.Head()
	targetSlot, targetParent, ok := s.BlockSlotAndParent(targetRoot)
	if !ok {
		return &types.Checkpoint{}
	}

	safeTargetSlot := uint64(0)
	if slot, _, ok := s.BlockSlotAndParent(s.SafeTarget()); ok {
		safeTargetSlot = slot
	}

	// The walk never crosses the finalized boundary: a safe target lagging
	// behind finalization falls back to the finalized slot as the lower bound.
	finalizedSlot := s.LatestFinalized().Slot
	lowerBoundSlot := safeTargetSlot
	if finalizedSlot > lowerBoundSlot {
		lowerBoundSlot = finalizedSlot
	}

	for range uint64(types.JustificationLookbackSlots) {
		if targetSlot <= lowerBoundSlot {
			break
		}
		slot, parent, ok := s.BlockSlotAndParent(targetParent)
		if !ok {
			targetRoot = targetParent
			break
		}
		targetRoot, targetSlot, targetParent = targetParent, slot, parent
	}

	for targetSlot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetSlot, finalizedSlot) {
		slot, parent, ok := s.BlockSlotAndParent(targetParent)
		if !ok {
			targetRoot = targetParent
			break
		}
		targetRoot, targetSlot, targetParent = targetParent, slot, parent
	}

	return &types.Checkpoint{
		Root: targetRoot,
		Slot: targetSlot,
	}
}
