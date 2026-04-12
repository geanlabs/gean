package forkchoice

import "github.com/geanlabs/gean/types"

// VoteTracker tracks per-validator attestation targets for delta computation.

type VoteTracker struct {
	AppliedIndex int // index of last applied vote, -1 if none
	LatestKnown  *VoteTarget
	LatestNew    *VoteTarget
}

// VoteTarget is a resolved attestation pointing to a proto-array index.
type VoteTarget struct {
	Index int // proto-array node index
	Slot  uint64
	Data  *types.AttestationData
}

// VoteStore holds per-validator vote trackers.
type VoteStore struct {
	Votes map[uint64]*VoteTracker // validator_id -> tracker
}

// NewVoteStore creates an empty vote store.
func NewVoteStore() *VoteStore {
	return &VoteStore{Votes: make(map[uint64]*VoteTracker)}
}

// SetKnown records a known (on-chain) attestation for a validator.
func (vs *VoteStore) SetKnown(validatorID uint64, nodeIndex int, slot uint64, data *types.AttestationData) {
	tracker := vs.getOrCreate(validatorID)
	tracker.LatestKnown = &VoteTarget{Index: nodeIndex, Slot: slot, Data: data}
}

// SetNew records a new (gossip-received) attestation for a validator.
func (vs *VoteStore) SetNew(validatorID uint64, nodeIndex int, slot uint64, data *types.AttestationData) {
	tracker := vs.getOrCreate(validatorID)
	tracker.LatestNew = &VoteTarget{Index: nodeIndex, Slot: slot, Data: data}
}

// PromoteNewToKnown moves all new votes to known.
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

// RemapIndices adjusts all vote tracker indices after proto-array pruning.
// Indices pointing to pruned nodes (< offset) are invalidated (-1 / nil).
// Surviving indices are shifted by -offset to match the new array layout.
// Cross-ref: zeam forkchoice.zig rebase (lines 760-796)
func (vs *VoteStore) RemapIndices(offset int, newLen int) {
	for _, tracker := range vs.Votes {
		// Remap AppliedIndex.
		if tracker.AppliedIndex >= 0 {
			newIdx := tracker.AppliedIndex - offset
			if newIdx < 0 || newIdx >= newLen {
				tracker.AppliedIndex = -1
			} else {
				tracker.AppliedIndex = newIdx
			}
		}

		// Remap LatestKnown.
		if tracker.LatestKnown != nil {
			newIdx := tracker.LatestKnown.Index - offset
			if newIdx < 0 || newIdx >= newLen {
				tracker.LatestKnown = nil
			} else {
				tracker.LatestKnown.Index = newIdx
			}
		}

		// Remap LatestNew.
		if tracker.LatestNew != nil {
			newIdx := tracker.LatestNew.Index - offset
			if newIdx < 0 || newIdx >= newLen {
				tracker.LatestNew = nil
			} else {
				tracker.LatestNew.Index = newIdx
			}
		}
	}
}

// ComputeDeltas computes weight deltas from vote changes.

// For each validator:
//   - Remove weight from previously applied index (if any)
//   - Add weight to current target index (from known or new pool)
//
// Each validator has weight 1.
func ComputeDeltas(numNodes int, votes *VoteStore, fromKnown bool) []int64 {
	deltas := make([]int64, numNodes)

	for _, tracker := range votes.Votes {
		// Remove previous vote.
		if tracker.AppliedIndex >= 0 && tracker.AppliedIndex < numNodes {
			deltas[tracker.AppliedIndex]--
		}
		tracker.AppliedIndex = -1

		// Apply current vote.
		var target *VoteTarget
		if fromKnown {
			target = tracker.LatestKnown
		} else {
			target = tracker.LatestNew
		}

		if target != nil && target.Index < numNodes {
			deltas[target.Index]++
			tracker.AppliedIndex = target.Index
		}
	}

	return deltas
}
