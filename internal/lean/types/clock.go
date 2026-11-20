package types

func CurrentSlot(genesisTime, currentTimeMs uint64) uint64 {
	return elapsedMs(genesisTime, currentTimeMs) / MillisecondsPerSlot
}

// BeyondFutureHorizon reports whether a block at slot is more than one slot past
// currentSlot. The full-slot margin still admits an intended early block.
func BeyondFutureHorizon(slot, currentSlot uint64) bool {
	return slot > currentSlot+1
}

// MillisIntoSlot returns how far the clock has advanced into the current slot.
func MillisIntoSlot(genesisTime, currentTimeMs uint64) uint64 {
	return elapsedMs(genesisTime, currentTimeMs) % MillisecondsPerSlot
}

func CurrentInterval(genesisTime, currentTimeMs uint64) uint64 {
	return MillisIntoSlot(genesisTime, currentTimeMs) / MillisecondsPerInterval
}

// NextIntervalBoundaryMs returns the unix-millisecond time of the first
// interval boundary strictly after currentTimeMs. Exactly on a boundary, that
// is the following one, a full interval away. Before genesis it is genesis
// itself. ok is false when the boundary does not fit in a uint64.
func NextIntervalBoundaryMs(genesisTime, currentTimeMs uint64) (uint64, bool) {
	genesisMs, ok := unixMillis(genesisTime)
	if !ok {
		return 0, false
	}
	if currentTimeMs < genesisMs {
		return genesisMs, true
	}
	elapsedIntervals := (currentTimeMs - genesisMs) / MillisecondsPerInterval
	if elapsedIntervals >= ^uint64(0)/MillisecondsPerInterval {
		return 0, false
	}
	offset := (elapsedIntervals + 1) * MillisecondsPerInterval
	if offset > ^uint64(0)-genesisMs {
		return 0, false
	}
	return genesisMs + offset, true
}

func TotalIntervals(genesisTime, currentTimeMs uint64) uint64 {
	return elapsedMs(genesisTime, currentTimeMs) / MillisecondsPerInterval
}

// elapsedMs is how far currentTimeMs is past genesis: zero before genesis, and
// when genesis does not fit in unix milliseconds.
func elapsedMs(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs, ok := unixMillis(genesisTime)
	if !ok || currentTimeMs < genesisMs {
		return 0
	}
	return currentTimeMs - genesisMs
}

func unixMillis(seconds uint64) (uint64, bool) {
	if seconds > ^uint64(0)/1000 {
		return 0, false
	}
	return seconds * 1000, true
}
