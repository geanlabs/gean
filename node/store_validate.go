package node

import (
	"math"

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

	// 9. Time: attestation's start interval must not exceed store time by
	// more than GossipDisparityIntervals (clock-skew tolerance, ~800ms).
	// Per leanSpec PR #682, the bound is in intervals, not slots — a whole-slot
	// margin would let an adversary pre-publish next-slot aggregates ahead of
	// any honest validator. The first conjunct guards against uint64 overflow
	// for malicious slot values near MaxUint64.
	if data.Slot > math.MaxUint64/types.IntervalsPerSlot ||
		data.Slot*types.IntervalsPerSlot > s.Time()+types.GossipDisparityIntervals {
		return errAttestationTooFarInFuture(data.Slot, s.Time())
	}

	return nil
}
