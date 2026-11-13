package chain

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
	"github.com/geanlabs/gean/lean/internal/store"
)

// OnTick advances the persisted interval clock and promotes live payloads in
// interval order. A failed time write stops before that interval's promotion.
func OnTick(
	s *store.ConsensusStore,
	p *pools.Attestations,
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
				p.PromoteNewToKnown()
			}
		case 4:
			p.PromoteNewToKnown()
		}
	}
}
