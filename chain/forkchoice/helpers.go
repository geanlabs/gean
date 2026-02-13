package forkchoice

import "github.com/devylongs/gean/types"

func containsVote(list []*types.SignedVote, sv *types.SignedVote) bool {
	for _, existing := range list {
		if existing.Data.ValidatorID == sv.Data.ValidatorID &&
			existing.Data.Slot == sv.Data.Slot {
			return true
		}
	}
	return false
}

func ceilDiv(a, b uint64) uint64 {
	return (a + b - 1) / b
}
