package attestation

import (
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func GetAttestationTarget(s *store.ConsensusStore) *types.Checkpoint {
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

	for range uint64(types.JustificationLookbackSlots) {
		if targetHeader.Slot <= safeTargetSlot {
			break
		}
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}

	finalizedSlot := s.LatestFinalized().Slot
	for targetHeader.Slot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetHeader.Slot, finalizedSlot) {
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}

	latestJustified := s.LatestJustified()
	if targetHeader.Slot < latestJustified.Slot {
		return latestJustified
	}

	return &types.Checkpoint{
		Root: targetRoot,
		Slot: targetHeader.Slot,
	}
}
