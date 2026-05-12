package types

// Pure slot-clock derivations matching leanSpec subspecs/chain/clock.py.
// Engine methods (node/tick.go) carry the runtime state; these stateless
// equivalents are used by spec conformance tests and by anyone needing the
// derivation without an Engine handle.

// CurrentSlot returns the slot number at currentTimeMs given genesisTime
// (seconds since Unix epoch). Returns 0 before genesis.
func CurrentSlot(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs := genesisTime * 1000
	if currentTimeMs < genesisMs {
		return 0
	}
	return (currentTimeMs - genesisMs) / MillisecondsPerSlot
}

// CurrentInterval returns the interval index within the current slot
// (0..IntervalsPerSlot-1). Returns 0 before genesis.
func CurrentInterval(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs := genesisTime * 1000
	if currentTimeMs < genesisMs {
		return 0
	}
	msIntoSlot := (currentTimeMs - genesisMs) % MillisecondsPerSlot
	return msIntoSlot / MillisecondsPerInterval
}

// TotalIntervals returns the total interval count since genesis (matches
// Store.time). Returns 0 before genesis.
func TotalIntervals(genesisTime, currentTimeMs uint64) uint64 {
	genesisMs := genesisTime * 1000
	if currentTimeMs < genesisMs {
		return 0
	}
	return (currentTimeMs - genesisMs) / MillisecondsPerInterval
}

// IntervalsFromSlot returns the interval count at the start of the given slot.
func IntervalsFromSlot(slot uint64) uint64 {
	return slot * IntervalsPerSlot
}

// IntervalsFromUnixTime returns the total interval count at the given Unix
// timestamp (seconds) relative to genesisTime. Returns 0 if the timestamp
// precedes genesis.
func IntervalsFromUnixTime(unixSeconds, genesisTime uint64) uint64 {
	if unixSeconds < genesisTime {
		return 0
	}
	return (unixSeconds - genesisTime) * 1000 / MillisecondsPerInterval
}
