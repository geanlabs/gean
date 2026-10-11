package statetransition

import "math/bits"

// SlotIsJustifiableAfter reports whether slot may be justified given the
// finalized slot, following leanSpec's slot.is_justifiable_after. It uses
// integer arithmetic only: the spec relies on math.isqrt, and floating point
// would round wrong above 2^53 and has no hardware support inside zkVM guests.
func SlotIsJustifiableAfter(slot, finalizedSlot uint64) bool {
	if slot < finalizedSlot {
		return false
	}
	delta := slot - finalizedSlot

	if delta <= 5 {
		return true
	}

	root := isqrt(delta)
	if root*root == delta {
		return true
	}

	// Pronic distances n(n+1). The spec tests whether 4*delta+1 is an odd
	// perfect square; for delta = n(n+1), isqrt(delta) = n, so checking
	// root*(root+1) is equivalent and cannot overflow (root < 2^32).
	return root*(root+1) == delta
}

// isqrt returns floor(sqrt(n)) using Newton's method from an upper bound.
func isqrt(n uint64) uint64 {
	if n < 2 {
		return n
	}
	x := uint64(1) << ((bits.Len64(n) + 1) / 2)
	for {
		y := (x + n/x) / 2
		if y >= x {
			return x
		}
		x = y
	}
}
