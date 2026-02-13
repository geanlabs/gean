package statetransition

// IsProposer checks if a validator is the proposer for a given slot using
// round-robin selection: slot % numValidators == validatorIndex.
func IsProposer(validatorIndex, slot, numValidators uint64) bool {
	if numValidators == 0 {
		panic("numValidators must be > 0")
	}
	return slot%numValidators == validatorIndex
}
