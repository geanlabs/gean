package node

import (
	"github.com/geanlabs/gean/types"
)

// ValidateAttestationData checks 9 validation branches for incoming attestations.
func ValidateAttestationData(s *ConsensusStore, data *types.AttestationData) error {
	// 1-3. Availability: source, target, head blocks must exist.
	sourceHeader := s.GetBlockHeader(data.Source.Root)
	if sourceHeader == nil {
		return errUnknownSourceBlock(data.Source.Root)
	}
	targetHeader := s.GetBlockHeader(data.Target.Root)
	if targetHeader == nil {
		return errUnknownTargetBlock(data.Target.Root)
	}
	headHeader := s.GetBlockHeader(data.Head.Root)
	if headHeader == nil {
		return errUnknownHeadBlock(data.Head.Root)
	}

	// 4. Topology: source.slot <= target.slot.
	if data.Source.Slot > data.Target.Slot {
		return errSourceExceedsTarget()
	}

	// 5. Topology: head.slot >= target.slot.
	if data.Head.Slot < data.Target.Slot {
		return errHeadOlderThanTarget(data.Head.Slot, data.Target.Slot)
	}

	// 6-8. Consistency: checkpoint slots match actual block slots.
	if sourceHeader.Slot != data.Source.Slot {
		return errSourceSlotMismatch(data.Source.Slot, sourceHeader.Slot)
	}
	if targetHeader.Slot != data.Target.Slot {
		return errTargetSlotMismatch(data.Target.Slot, targetHeader.Slot)
	}
	if headHeader.Slot != data.Head.Slot {
		return errHeadSlotMismatch(data.Head.Slot, headHeader.Slot)
	}

	// 9. Time: attestation not > 1 slot in future.
	currentSlot := s.Time() / types.IntervalsPerSlot
	if data.Slot > currentSlot+1 {
		return errAttestationTooFarInFuture(data.Slot, currentSlot)
	}

	return nil
}
