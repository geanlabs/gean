package types

// Primitive Types

type Slot uint64
type ValidatorIndex uint64
type Root [32]byte

func (r Root) IsZero() bool { return r == Root{} }

// IsJustifiableAfter checks if this slot is a valid candidate for justification
// after the given finalized slot. Per 3SF-mini spec:
// - delta <= 5 (immediate)
// - delta is a perfect square (x^2)
// - delta is a pronic number (x^2 + x)
func (s Slot) IsJustifiableAfter(finalizedSlot Slot) bool {
	if s < finalizedSlot {
		return false
	}
	delta := int(s - finalizedSlot)
	if delta <= 5 {
		return true
	}
	// Check perfect square
	sqrt := int(float64(delta) + 0.5)
	for sqrt*sqrt > delta {
		sqrt--
	}
	if sqrt*sqrt == delta {
		return true
	}
	// Check pronic (x^2 + x = x*(x+1))
	for x := 1; x*(x+1) <= delta; x++ {
		if x*(x+1) == delta {
			return true
		}
	}
	return false
}

// Constants

const (
	SecondsPerSlot             uint64 = 4
	IntervalsPerSlot           uint64 = 4
	SecondsPerInterval         uint64 = SecondsPerSlot / IntervalsPerSlot
	JustificationLookbackSlots uint64 = 3
	HistoricalRootsLimit       uint64 = 1 << 18 // 262144
	ValidatorRegistryLimit     uint64 = 1 << 12 // 4096
)

// Time Helpers

func (s Slot) Time(genesisTime uint64) uint64 {
	return genesisTime + uint64(s)*SecondsPerSlot
}

func SlotAt(time, genesisTime uint64) Slot {
	if time < genesisTime {
		return 0
	}
	return Slot((time - genesisTime) / SecondsPerSlot)
}

func IntervalAt(time, genesisTime uint64) uint64 {
	if time < genesisTime {
		return 0
	}
	offset := (time - genesisTime) % SecondsPerSlot
	return offset / SecondsPerInterval
}
