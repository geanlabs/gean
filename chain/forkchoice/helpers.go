package forkchoice

import "github.com/geanlabs/gean/types"

func containsAttestation(list []*types.Attestation, att *types.Attestation) bool {
	for _, existing := range list {
		if existing.ValidatorID == att.ValidatorID &&
			existing.Data.Slot == att.Data.Slot {
			return true
		}
	}
	return false
}

func ceilDiv(a, b uint64) uint64 {
	return (a + b - 1) / b
}
