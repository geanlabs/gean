package blockbuilder

import "github.com/geanlabs/gean/internal/types"

func requiredCheckpoint(checkpoint *types.Checkpoint) *types.Checkpoint {
	if checkpoint != nil {
		return checkpoint
	}
	return &types.Checkpoint{}
}

func justifiedMeetsRequired(state *types.State, required *types.Checkpoint) bool {
	if required == nil {
		return true
	}
	var actual *types.Checkpoint
	if state != nil {
		actual = state.LatestJustified
	}
	if actual == nil {
		return required.Slot == 0 && types.IsZeroRoot(required.Root)
	}
	if actual.Slot != required.Slot {
		if actual.Slot < required.Slot {
			return false
		}
		return types.IsZeroRoot(required.Root) || checkpointInHistory(state, required)
	}
	return types.IsZeroRoot(required.Root) || actual.Root == required.Root
}

func checkpointInHistory(state *types.State, checkpoint *types.Checkpoint) bool {
	if state == nil || checkpoint == nil || checkpoint.Slot >= uint64(len(state.HistoricalBlockHashes)) {
		return false
	}
	rootBytes := state.HistoricalBlockHashes[checkpoint.Slot]
	if len(rootBytes) != types.RootSize {
		return false
	}
	var root [32]byte
	copy(root[:], rootBytes)
	return root == checkpoint.Root
}
