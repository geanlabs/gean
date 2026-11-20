package types

var ZeroRoot [RootSize]byte

func IsZeroRoot(root [RootSize]byte) bool {
	return root == ZeroRoot
}

func ProposerIndex(slot, numValidators uint64) uint64 {
	if numValidators == 0 {
		return 0
	}
	return slot % numValidators
}

func IsProposer(slot, validatorIndex, numValidators uint64) bool {
	if numValidators == 0 {
		return false
	}
	return ProposerIndex(slot, numValidators) == validatorIndex
}
