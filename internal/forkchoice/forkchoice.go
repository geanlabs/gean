package forkchoice

import (
	"slices"
	"sync/atomic"

	"github.com/geanlabs/gean/internal/types"
)

type ForkChoice struct {
	array *ProtoArray
	votes *VoteStore
	// nodes is an immutable copy of the array for readers off the dispatch loop,
	// replaced after each head update and prune so it always carries head weights.
	nodes atomic.Pointer[[]ProtoNode]
}

func New(anchorSlot uint64, anchorRoot, anchorParentRoot [32]byte) *ForkChoice {
	fc := &ForkChoice{
		array: NewProtoArray(anchorSlot, anchorRoot, anchorParentRoot),
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
	if fc == nil || fc.array == nil {
		return
	}
	fc.array.OnBlock(slot, root, parentRoot)
}

func (fc *ForkChoice) UpdateHead(justifiedRoot [32]byte) [32]byte {
	if fc == nil || fc.array == nil {
		return justifiedRoot
	}
	deltas := ComputeDeltas(fc.array.Len(), fc.votes, true)
	fc.array.ApplyScoreChanges(deltas, 0)
	head := fc.array.FindHead(justifiedRoot)
	fc.publishNodes()
	return head
}

func (fc *ForkChoice) UpdateSafeTarget(justifiedRoot [32]byte, numValidators uint64) [32]byte {
	if fc == nil || fc.array == nil {
		return justifiedRoot
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
	if fc == nil || fc.array == nil {
		return
	}
	finalizedIdx, ok := fc.array.indices[finalizedRoot]
	if !ok || finalizedIdx == 0 {
		return
	}

	indexMap := fc.array.Prune(finalizedRoot)

	if fc.votes != nil && indexMap != nil {
		fc.votes.RemapIndices(indexMap)
	}
	fc.publishNodes()
}

func (fc *ForkChoice) NodeIndex(root [32]byte) int {
	if fc == nil || fc.array == nil {
		return -1
	}
	if idx, ok := fc.array.indices[root]; ok {
		return idx
	}
	return -1
}

func (fc *ForkChoice) Len() int {
	if fc == nil || fc.array == nil {
		return 0
	}
	return fc.array.Len()
}

// Nodes returns a copy of the nodes as of the last head update or prune. It is safe
// to call off the dispatch loop.
func (fc *ForkChoice) Nodes() []ProtoNode {
	if fc == nil {
		return nil
	}
	nodes := fc.nodes.Load()
	if nodes == nil {
		return nil
	}
	return slices.Clone(*nodes)
}

func (fc *ForkChoice) SetKnownVote(validatorID uint64, headRoot [32]byte, slot uint64, data *types.AttestationData) bool {
	if fc == nil || fc.votes == nil {
		return false
	}
	idx := fc.NodeIndex(headRoot)
	if idx < 0 {
		return false
	}
	fc.votes.SetKnown(validatorID, idx, slot, data)
	return true
}

func (fc *ForkChoice) SetNewVote(validatorID uint64, headRoot [32]byte, slot uint64, data *types.AttestationData) bool {
	if fc == nil || fc.votes == nil {
		return false
	}
	idx := fc.NodeIndex(headRoot)
	if idx < 0 {
		return false
	}
	fc.votes.SetNew(validatorID, idx, slot, data)
	return true
}

func (fc *ForkChoice) VoteTracker(validatorID uint64) (*VoteTracker, bool) {
	if fc == nil || fc.votes == nil {
		return nil, false
	}
	tracker, ok := fc.votes.Votes[validatorID]
	if !ok {
		return nil, false
	}
	return copyVoteTracker(tracker), true
}
