package forkchoice

import "github.com/devylongs/gean/types"

// CurrentSlot returns the current slot based on store time.
func (s *Store) CurrentSlot() types.Slot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Clock.CurrentSlot()
}

// CurrentInterval returns the current interval within the slot.
func (s *Store) CurrentInterval() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Clock.CurrentInterval()
}

// TickInterval advances store time by one interval.
func (s *Store) TickInterval(hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	interval := s.Clock.Tick()
	s.onIntervalLocked(interval, hasProposal)
}

func (s *Store) onIntervalLocked(interval uint64, hasProposal bool) {
	switch interval {
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

// AdvanceTime ticks the store forward to the given unix time.
func (s *Store) AdvanceTime(unixTime uint64, hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target := s.Clock.TargetIntervals(unixTime)
	for s.Clock.Intervals() < target {
		isLast := s.Clock.Intervals()+1 == target
		interval := s.Clock.Tick()
		s.onIntervalLocked(interval, hasProposal && isLast)
	}
}

func (s *Store) advanceToSlotLocked(slot types.Slot) {
	targetIntervals := uint64(slot) * types.IntervalsPerSlot
	for s.Clock.Intervals() < targetIntervals {
		isLast := s.Clock.Intervals()+1 == targetIntervals
		interval := s.Clock.Tick()
		s.onIntervalLocked(interval, isLast)
	}
	s.acceptNewVotesLocked()
}
