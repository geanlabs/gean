package forkchoice

func copyVoteTracker(tracker *VoteTracker) *VoteTracker {
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
		Data:  target.Data.Copy(),
	}
}
