package statetransition

import "github.com/geanlabs/gean/internal/types"

func tryFinalize(
	state *types.State,
	source, target *types.Checkpoint,
	justifications *map[[32]byte][]bool,
	rootToSlot map[[32]byte]uint64,
) {
	if state == nil || state.LatestFinalized == nil || source == nil || target == nil || justifications == nil {
		return
	}

	finalizedSlot := state.LatestFinalized.Slot
	if source.Slot <= finalizedSlot {
		return
	}

	for slot := source.Slot + 1; slot < target.Slot; slot++ {
		if SlotIsJustifiableAfter(slot, state.LatestFinalized.Slot) {
			return
		}
	}

	state.LatestFinalized = copyCheckpoint(source)
	shiftJustifiedSlots(state, state.LatestFinalized.Slot-finalizedSlot)

	for root := range *justifications {
		slot, found := rootToSlot[root]
		if !found || slot <= state.LatestFinalized.Slot {
			delete(*justifications, root)
		}
	}
}

func shiftJustifiedSlots(state *types.State, delta uint64) {
	if state == nil || delta == 0 {
		return
	}
	oldLen := types.BitlistLen(state.JustifiedSlots)
	if delta >= oldLen {
		state.JustifiedSlots = types.NewBitlistSSZ(0)
		return
	}
	newLen := oldLen - delta
	newBits := types.NewBitlistSSZ(newLen)
	for i := range newLen {
		if types.BitlistGet(state.JustifiedSlots, i+delta) {
			types.BitlistSet(newBits, i)
		}
	}
	state.JustifiedSlots = newBits
}

func buildRootToSlot(state *types.State) map[[32]byte]uint64 {
	rootToSlot := make(map[[32]byte]uint64)
	if state == nil || state.LatestFinalized == nil {
		return rootToSlot
	}
	for slot := state.LatestFinalized.Slot + 1; slot < uint64(len(state.HistoricalBlockHashes)); slot++ {
		var root [32]byte
		copy(root[:], state.HistoricalBlockHashes[slot])
		if existing, ok := rootToSlot[root]; !ok || slot > existing {
			rootToSlot[root] = slot
		}
	}
	return rootToSlot
}
