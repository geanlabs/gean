package statetransition

// justifiedIndexAfter returns the relative justified-slots index for targetSlot.
// Slots at or before finalizedSlot are considered implicitly justified and have no index.
func justifiedIndexAfter(finalizedSlot, targetSlot uint64) (uint64, bool) {
	if targetSlot <= finalizedSlot {
		return 0, false
	}
	return targetSlot - finalizedSlot - 1, true
}

// isSlotJustified checks justification status using the finalized-relative slot window.
func isSlotJustified(justifiedSlots []byte, finalizedSlot, targetSlot uint64) bool {
	relativeIndex, ok := justifiedIndexAfter(finalizedSlot, targetSlot)
	if !ok {
		return true
	}
	if relativeIndex >= uint64(BitlistLen(justifiedSlots)) {
		return false
	}
	return GetBit(justifiedSlots, relativeIndex)
}

// extendJustifiedSlotsToSlot ensures justifiedSlots can represent targetSlot.
// New entries are initialized to false.
func extendJustifiedSlotsToSlot(justifiedSlots []byte, finalizedSlot, targetSlot uint64) []byte {
	relativeIndex, ok := justifiedIndexAfter(finalizedSlot, targetSlot)
	if !ok {
		return CloneBitlist(justifiedSlots)
	}

	out := CloneBitlist(justifiedSlots)
	for uint64(BitlistLen(out)) <= relativeIndex {
		out = AppendBit(out, false)
	}
	return out
}

// setSlotJustified updates the justified status for targetSlot if it is tracked.
func setSlotJustified(justifiedSlots []byte, finalizedSlot, targetSlot uint64, value bool) []byte {
	relativeIndex, ok := justifiedIndexAfter(finalizedSlot, targetSlot)
	if !ok {
		return CloneBitlist(justifiedSlots)
	}

	out := CloneBitlist(justifiedSlots)
	if relativeIndex >= uint64(BitlistLen(out)) {
		return out
	}
	return SetBit(out, relativeIndex, value)
}

// shiftJustifiedSlotsWindow drops delta entries from the head of the tracking window.
func shiftJustifiedSlotsWindow(justifiedSlots []byte, delta uint64) []byte {
	if delta == 0 {
		return CloneBitlist(justifiedSlots)
	}

	currentLen := uint64(BitlistLen(justifiedSlots))
	if delta >= currentLen {
		return []byte{0x01}
	}

	out := []byte{0x01}
	for i := delta; i < currentLen; i++ {
		out = AppendBit(out, GetBit(justifiedSlots, i))
	}
	return out
}
