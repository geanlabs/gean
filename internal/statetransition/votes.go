package statetransition

import "github.com/geanlabs/gean/internal/types"

func validAttestationShape(agg *types.AggregatedAttestation) bool {
	return agg != nil && agg.Data != nil && agg.Data.Source != nil && agg.Data.Target != nil
}

const (
	VoteReasonNilInput               = "nil_input"
	VoteReasonSourceNotJustified     = "source_not_justified"
	VoteReasonTargetAlreadyJustified = "target_already_justified"
	VoteReasonZeroRoot               = "zero_root"
	VoteReasonChainMismatch          = "chain_mismatch"
	VoteReasonTargetNotAfterSource   = "target_not_after_source"
	VoteReasonTargetNotJustifiable   = "target_not_justifiable"
)

func VoteInvalidReason(state *types.State, source, target *types.Checkpoint) string {
	if state == nil || state.LatestFinalized == nil || source == nil || target == nil {
		return VoteReasonNilInput
	}

	finalizedSlot := state.LatestFinalized.Slot
	if !IsSlotJustified(state, finalizedSlot, source.Slot) {
		return VoteReasonSourceNotJustified
	}
	if types.IsZeroRoot(source.Root) || types.IsZeroRoot(target.Root) {
		return VoteReasonZeroRoot
	}
	if !checkpointExists(state, source) || !checkpointExists(state, target) {
		return VoteReasonChainMismatch
	}
	if IsSlotJustified(state, finalizedSlot, target.Slot) {
		return VoteReasonTargetAlreadyJustified
	}
	if target.Slot <= source.Slot {
		return VoteReasonTargetNotAfterSource
	}
	if !SlotIsJustifiableAfter(target.Slot, finalizedSlot) {
		return VoteReasonTargetNotJustifiable
	}
	return ""
}

func IsValidVote(state *types.State, source, target *types.Checkpoint) bool {
	return VoteInvalidReason(state, source, target) == ""
}

// HeadMatchesChain reports whether the attestation head sits on the canonical
// chain at its slot. Mirrors the head clause of leanSpec attestation_data_matches_chain;
// applied both in process_attestations and in block production.
func HeadMatchesChain(state *types.State, head *types.Checkpoint) bool {
	return head != nil && !types.IsZeroRoot(head.Root) && checkpointExists(state, head)
}

func IsSlotJustified(state *types.State, finalizedSlot, slot uint64) bool {
	if slot <= finalizedSlot {
		return true
	}
	if state == nil {
		return false
	}
	relIndex := slot - finalizedSlot - 1
	return relIndex < types.BitlistLen(state.JustifiedSlots) &&
		types.BitlistGet(state.JustifiedSlots, relIndex)
}

func setSlotJustified(state *types.State, finalizedSlot, slot uint64) {
	if state == nil || slot <= finalizedSlot {
		return
	}
	relIndex := slot - finalizedSlot - 1
	if relIndex >= types.BitlistLen(state.JustifiedSlots) {
		state.JustifiedSlots = types.BitlistExtend(state.JustifiedSlots, relIndex+1)
	}
	types.BitlistSet(state.JustifiedSlots, relIndex)
}

func checkpointExists(state *types.State, cp *types.Checkpoint) bool {
	if state == nil || cp == nil {
		return false
	}
	if cp.Slot >= uint64(len(state.HistoricalBlockHashes)) {
		return false
	}
	var stored [32]byte
	copy(stored[:], state.HistoricalBlockHashes[cp.Slot])
	return stored == cp.Root
}

func countTrue(votes []bool) int {
	count := 0
	for _, v := range votes {
		if v {
			count++
		}
	}
	return count
}
