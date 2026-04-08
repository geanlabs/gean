package types

import "encoding/hex"

var ZeroRoot [RootSize]byte

// IsZeroRoot returns true if the root is all zeros.
func IsZeroRoot(root [RootSize]byte) bool {
	return root == ZeroRoot
}

// IsProposer returns true if validatorIndex is the proposer for the given slot.
func IsProposer(slot, validatorIndex, numValidators uint64) bool {
	if numValidators == 0 {
		return false
	}
	return slot%numValidators == validatorIndex
}

// ProposerIndex returns the proposer for a slot, or -1 if no validators.
func ProposerIndex(slot, numValidators uint64) int64 {
	if numValidators == 0 {
		return -1
	}
	return int64(slot % numValidators)
}

// ShortRoot returns the first 4 bytes of a root as hex for logging.
func ShortRoot(root [RootSize]byte) string {
	return "0x" + hex.EncodeToString(root[:4])
}
