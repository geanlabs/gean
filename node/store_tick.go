package node

import (
	"github.com/geanlabs/gean/types"
)

// OnTick processes a tick event, dispatching interval-specific actions.
//
// Returns any new aggregated attestations produced at interval 2.
// Note: head/safe-target updates are NOT done here — they happen in Engine
// which owns ForkChoice. This only handles payload promotion and aggregation.
func OnTick(
	s *ConsensusStore,
	timestampMs uint64,
	hasProposal bool,
	isAggregator bool,
) []*types.SignedAggregatedAttestation {
	var newAggregates []*types.SignedAggregatedAttestation

	// Convert UNIX timestamp (ms) to interval count since genesis.
	genesisTimeMs := s.Config().GenesisTime * 1000
	timeDeltaMs := timestampMs - genesisTimeMs
	if timestampMs < genesisTimeMs {
		timeDeltaMs = 0
	}
	time := timeDeltaMs / types.MillisecondsPerInterval

	// Fast-forward if more than a slot behind.
	// Use guard to prevent uint64 underflow.
	if time > s.Time() && time-s.Time() > types.IntervalsPerSlot {
		s.SetTime(time - types.IntervalsPerSlot)
	}

	for s.Time() < time {
		s.SetTime(s.Time() + 1)

		interval := s.Time() % types.IntervalsPerSlot

		// has_proposal only signaled for the final tick.
		isFinalTick := s.Time() == time
		shouldSignalProposal := hasProposal && isFinalTick

		switch interval {
		case 0:
			// Start of slot — promote attestations if a proposal exists.
			// When this node will propose, migrate new→known so the head
			// update + block-builder both read fresh known. Non-proposer
			// nodes do nothing here; their migration runs at interval 4.
			if shouldSignalProposal {
				s.PromoteNewToKnown()
			}
		case 1:
			// Vote propagation — no store action.
		case 2:
			// Aggregation interval — dispatched by Engine (see onTick).
		case 3:
			// Safe target update happens in Engine (it owns ForkChoice).
		case 4:
			// End of slot — promote accumulated attestations.
			s.PromoteNewToKnown()
			// Head update happens in Engine.
		}
	}

	return newAggregates
}
