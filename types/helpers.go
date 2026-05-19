package types

import "encoding/hex"

var ZeroRoot [RootSize]byte

// IsZeroRoot returns true if the root is all zeros.
func IsZeroRoot(root [RootSize]byte) bool {
	return root == ZeroRoot
}

// ProposerIndex returns the proposer index for a slot.
// Caller must ensure numValidators > 0.
func ProposerIndex(slot, numValidators uint64) uint64 {
	return slot % numValidators
}

// IsProposer returns true if validatorIndex is the proposer for the given slot.
func IsProposer(slot, validatorIndex, numValidators uint64) bool {
	if numValidators == 0 {
		return false
	}
	return ProposerIndex(slot, numValidators) == validatorIndex
}

// ShortRoot returns the first 4 bytes of a root as hex for logging.
func ShortRoot(root [RootSize]byte) string {
	return "0x" + hex.EncodeToString(root[:4])
}
