package types

import "encoding/hex"

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

func ShortRoot(root [RootSize]byte) string {
	return "0x" + hex.EncodeToString(root[:4])
}
