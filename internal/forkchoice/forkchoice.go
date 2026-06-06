package forkchoice

import "github.com/geanlabs/gean/internal/types"

type ForkChoice struct {
	array *ProtoArray
	votes *VoteStore
}

func New(anchorSlot uint64, anchorRoot, anchorParentRoot [32]byte) *ForkChoice {
	return &ForkChoice{
		array: NewProtoArray(anchorSlot, anchorRoot, anchorParentRoot),
		votes: NewVoteStore(),
	}
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
	return fc.array.FindHead(justifiedRoot)
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

func (fc *ForkChoice) Nodes() []ProtoNode {
	if fc == nil || fc.array == nil {
		return nil
	}
	return fc.array.Nodes()
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
