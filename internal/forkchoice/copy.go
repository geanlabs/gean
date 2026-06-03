package forkchoice

import "github.com/geanlabs/gean/internal/types"

func copyVoteTracker(tracker *VoteTracker) *VoteTracker {
	if tracker == nil {
		return nil
	}
	return &VoteTracker{
		AppliedIndex: tracker.AppliedIndex,
		LatestKnown:  copyVoteTarget(tracker.LatestKnown),
		LatestNew:    copyVoteTarget(tracker.LatestNew),
	}
}

func copyVoteTarget(target *VoteTarget) *VoteTarget {
	if target == nil {
		return nil
	}
	return &VoteTarget{
		Index: target.Index,
		Slot:  target.Slot,
		Data:  copyAttestationData(target.Data),
	}
}

func copyAttestationData(data *types.AttestationData) *types.AttestationData {
	if data == nil {
		return nil
	}
	return &types.AttestationData{
		Slot:   data.Slot,
		Head:   copyCheckpoint(data.Head),
		Target: copyCheckpoint(data.Target),
		Source: copyCheckpoint(data.Source),
	}
}

func copyCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return nil
	}
	out := *cp
	return &out
}
