package store

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func OnTick(
	s *ConsensusStore,
	timestampMs uint64,
	hasProposal bool,
) {
	if s == nil {
		return
	}
	genesisTimeMs := s.Config().GenesisTime * 1000
	var timeDeltaMs uint64
	if timestampMs > genesisTimeMs {
		timeDeltaMs = timestampMs - genesisTimeMs
	}
	time := timeDeltaMs / types.MillisecondsPerInterval

	storeTime := s.Time()
	if time > storeTime && time-storeTime > types.IntervalsPerSlot {
		storeTime = time - types.IntervalsPerSlot
		if err := s.PutTime(storeTime); err != nil {
			logger.Error(logger.Store, "tick: advance time failed: %v", err)
			return
		}
	}

	for storeTime < time {
		storeTime++
		if err := s.PutTime(storeTime); err != nil {
			logger.Error(logger.Store, "tick: advance time failed: %v", err)
			return
		}

		interval := storeTime % types.IntervalsPerSlot

		isFinalTick := storeTime == time
		shouldSignalProposal := hasProposal && isFinalTick

		switch interval {
		case 0:
			if shouldSignalProposal {
				s.PromoteNewToKnown()
			}
		case 4:
			s.PromoteNewToKnown()
		}
	}
}
