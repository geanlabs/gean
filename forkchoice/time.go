package forkchoice

import "github.com/devylongs/gean/types"

// CurrentSlot returns the current slot based on store time.
func (s *Store) CurrentSlot() types.Slot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return types.Slot(s.Time / types.IntervalsPerSlot)
}

// CurrentInterval returns the current interval within the slot.
func (s *Store) CurrentInterval() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Time % types.IntervalsPerSlot
}

// TickInterval advances store time by one interval.
func (s *Store) TickInterval(hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickIntervalLocked(hasProposal)
}

func (s *Store) tickIntervalLocked(hasProposal bool) {
	s.Time++
	currentInterval := s.Time % types.IntervalsPerSlot

	switch currentInterval {
	case 0:
		if hasProposal {
			s.acceptNewVotesLocked()
		}
	case 1:
		// Validator voting interval - no action
	case 2:
		s.updateSafeTargetLocked()
	default:
		s.acceptNewVotesLocked()
	}
}

// AdvanceTime ticks the store forward to the given time.
func (s *Store) AdvanceTime(time uint64, hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Don't advance time if we're before genesis
	if time < s.Config.GenesisTime {
		return
	}

	tickIntervalTime := (time - s.Config.GenesisTime) / types.SecondsPerInterval

	for s.Time < tickIntervalTime {
		shouldSignal := hasProposal && (s.Time+1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
}

func (s *Store) advanceToSlotLocked(slot types.Slot) {
	slotTime := s.Config.GenesisTime + uint64(slot)*types.SecondsPerSlot
	tickIntervalTime := (slotTime - s.Config.GenesisTime) / types.SecondsPerInterval
	for s.Time < tickIntervalTime {
		shouldSignal := (s.Time + 1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
	s.acceptNewVotesLocked()
}
