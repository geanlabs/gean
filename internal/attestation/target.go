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
			break
		}
		targetRoot, targetSlot, targetParent = targetParent, slot, parent
	}

	for targetSlot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetSlot, finalizedSlot) {
		slot, parent, ok := s.BlockSlotAndParent(targetParent)
		if !ok {
			break
		}
		targetRoot, targetSlot, targetParent = targetParent, slot, parent
	}

	// The root and slot always describe the same block, as the spec's
	// Checkpoint(root=target_block_root, slot=target_block.slot) does. When a
	// parent is missing the walk stops on the last block it has, rather than
	// naming the missing parent with its child's slot, a checkpoint no peer
	// could accept.
	return &types.Checkpoint{
		Root: targetRoot,
		Slot: targetSlot,
	}
}
