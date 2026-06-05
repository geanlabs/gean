package statetransition

import "math"

func SlotIsJustifiableAfter(slot, finalizedSlot uint64) bool {
	if slot < finalizedSlot {
		return false
	}
	delta := slot - finalizedSlot

	if delta <= 5 {
		return true
	}

	sqrt := uint64(math.Sqrt(float64(delta)))
	if sqrt*sqrt == delta {
		return true
	}

	val := 4*delta + 1
	sqrtVal := uint64(math.Sqrt(float64(val)))
	if sqrtVal*sqrtVal == val && sqrtVal%2 == 1 {
		return true
	}

	return false
}
