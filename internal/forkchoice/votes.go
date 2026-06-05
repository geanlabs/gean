package forkchoice

import "github.com/geanlabs/gean/internal/types"

type VoteTracker struct {
	AppliedIndex int
	LatestKnown  *VoteTarget
	LatestNew    *VoteTarget
}

type VoteTarget struct {
	Index int
	Slot  uint64
	Data  *types.AttestationData
}

type VoteStore struct {
	Votes map[uint64]*VoteTracker
}

func NewVoteStore() *VoteStore {
	return &VoteStore{Votes: make(map[uint64]*VoteTracker)}
}

func (vs *VoteStore) SetKnown(validatorID uint64, nodeIndex int, slot uint64, data *types.AttestationData) {
	if nodeIndex < 0 {
		return
	}
	tracker := vs.getOrCreate(validatorID)
	tracker.LatestKnown = &VoteTarget{Index: nodeIndex, Slot: slot, Data: data}
}

func (vs *VoteStore) SetNew(validatorID uint64, nodeIndex int, slot uint64, data *types.AttestationData) {
	if nodeIndex < 0 {
		return
	}
	tracker := vs.getOrCreate(validatorID)
	tracker.LatestNew = &VoteTarget{Index: nodeIndex, Slot: slot, Data: data}
}

func (vs *VoteStore) PromoteNewToKnown() {
	for _, tracker := range vs.Votes {
		if tracker.LatestNew != nil {
			tracker.LatestKnown = tracker.LatestNew
			tracker.LatestNew = nil
		}
	}
}

func (vs *VoteStore) getOrCreate(validatorID uint64) *VoteTracker {
	t, ok := vs.Votes[validatorID]
	if !ok {
		t = &VoteTracker{AppliedIndex: -1}
		vs.Votes[validatorID] = t
	}
	return t
}

func (vs *VoteStore) RemapIndices(indexMap map[int]int) {
	if vs == nil || indexMap == nil {
		return
	}
	for _, tracker := range vs.Votes {
		if tracker == nil {
			continue
		}
		if tracker.AppliedIndex >= 0 {
			tracker.AppliedIndex = remapVoteIndex(tracker.AppliedIndex, indexMap)
		}

		if tracker.LatestKnown != nil {
			newIdx := remapVoteIndex(tracker.LatestKnown.Index, indexMap)
			if newIdx < 0 {
				tracker.LatestKnown = nil
			} else {
				tracker.LatestKnown.Index = newIdx
			}
		}

		if tracker.LatestNew != nil {
			newIdx := remapVoteIndex(tracker.LatestNew.Index, indexMap)
			if newIdx < 0 {
				tracker.LatestNew = nil
			} else {
				tracker.LatestNew.Index = newIdx
			}
		}
	}
}

func remapVoteIndex(oldIdx int, indexMap map[int]int) int {
	if oldIdx < 0 {
		return -1
	}
	if newIdx, ok := indexMap[oldIdx]; ok {
		return newIdx
	}
	return -1
}

func ComputeDeltas(numNodes int, votes *VoteStore, fromKnown bool) []int64 {
	deltas := make([]int64, numNodes)
	if votes == nil {
		return deltas
	}

	for _, tracker := range votes.Votes {
		if tracker == nil {
			continue
		}
		if tracker.AppliedIndex >= 0 && tracker.AppliedIndex < numNodes {
			deltas[tracker.AppliedIndex]--
		}
		tracker.AppliedIndex = -1

		var target *VoteTarget
		if fromKnown {
			target = tracker.LatestKnown
		} else {
			target = tracker.LatestNew
		}

		if target != nil && target.Index >= 0 && target.Index < numNodes {
			deltas[target.Index]++
			tracker.AppliedIndex = target.Index
		}
	}

	return deltas
}
