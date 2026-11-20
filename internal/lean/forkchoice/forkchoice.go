package forkchoice

import (
	"slices"
	"sync/atomic"

	"github.com/geanlabs/gean/internal/core/protoarray"
	"github.com/geanlabs/gean/internal/lean/types"
)

// ProtoNode is one block in the fork-choice tree.
type ProtoNode = protoarray.Node

type ForkChoice struct {
	array *protoarray.Array
	votes *VoteStore
	// nodes is an immutable copy of the array for readers off the dispatch loop,
	// replaced after each head update and prune so it always carries head weights.
	nodes atomic.Pointer[[]ProtoNode]
}

func New(anchorSlot uint64, anchorRoot, anchorParentRoot [32]byte) *ForkChoice {
	fc := &ForkChoice{
		array: protoarray.New(anchorSlot, anchorRoot, anchorParentRoot),
		votes: NewVoteStore(),
	}
	fc.publishNodes()
	return fc
}

func (fc *ForkChoice) publishNodes() {
	nodes := fc.array.Nodes()
	fc.nodes.Store(&nodes)
}

func (fc *ForkChoice) OnBlock(slot uint64, root, parentRoot [32]byte) {
	fc.array.OnBlock(slot, root, parentRoot)
}

func (fc *ForkChoice) UpdateHead(justifiedRoot [32]byte) [32]byte {
	deltas := ComputeDeltas(fc.array.Len(), fc.votes, true)
	fc.array.ApplyScoreChanges(deltas, 0)
	head := fc.array.FindHead(justifiedRoot)
	fc.publishNodes()
	return head
}

// UpdateSafeTarget counts only the given votes: the latest per validator from the
// current new-payload pool. Every earlier new vote is dropped first, so a validator
// absent from this pool contributes nothing, as in leanSpec's update_safe_target.
func (fc *ForkChoice) UpdateSafeTarget(justifiedRoot [32]byte, numValidators uint64, votes map[uint64]*types.AttestationData) [32]byte {
	fc.votes.ClearNew()
	for vid, data := range votes {
		fc.votes.SetNew(vid, fc.NodeIndex(data.Head.Root), data.Slot, data)
	}
	if numValidators == 0 {
		return justifiedRoot
	}
	minScore := quorumScore(numValidators)
	deltas := ComputeDeltas(fc.array.Len(), fc.votes, false)
	fc.array.ApplyScoreChanges(deltas, minScore)
	return fc.array.FindHead(justifiedRoot)
}

func (fc *ForkChoice) Prune(finalizedRoot [32]byte) {
	finalizedIdx, ok := fc.array.Index(finalizedRoot)
	if !ok || finalizedIdx == 0 {
		return
	}

	fc.votes.RemapIndices(fc.array.Prune(finalizedRoot))
	fc.publishNodes()
}

func (fc *ForkChoice) NodeIndex(root [32]byte) int {
	if idx, ok := fc.array.Index(root); ok {
		return idx
	}
	return -1
}

func (fc *ForkChoice) Len() int {
	return fc.array.Len()
}

// Nodes returns a copy of the nodes as of the last head update or prune. It is safe
// to call off the dispatch loop.
func (fc *ForkChoice) Nodes() []ProtoNode {
	return slices.Clone(*fc.nodes.Load())
}

func (fc *ForkChoice) SetKnownVote(validatorID uint64, headRoot [32]byte, slot uint64, data *types.AttestationData) bool {
	idx := fc.NodeIndex(headRoot)
	if idx < 0 {
		return false
	}
	fc.votes.SetKnown(validatorID, idx, slot, data)
	return true
}

func (fc *ForkChoice) VoteTracker(validatorID uint64) (*VoteTracker, bool) {
	tracker, ok := fc.votes.Votes[validatorID]
	if !ok {
		return nil, false
	}
	return copyVoteTracker(tracker), true
}
