package node

import "fmt"

// StoreError represents errors during consensus store operations.
type StoreError struct {
	Kind    StoreErrorKind
	Message string
}

func (e *StoreError) Error() string { return e.Message }

type StoreErrorKind int

const (
	ErrMissingParentState StoreErrorKind = iota
	ErrInvalidValidatorIndex
	ErrPubkeyDecodingFailed
	ErrSignatureDecodingFailed
	ErrSignatureVerificationFailed
	ErrProposerSignatureDecodingFailed
	ErrProposerSignatureVerificationFailed
	ErrStateTransitionFailed
	ErrUnknownSourceBlock
	ErrUnknownTargetBlock
	ErrUnknownHeadBlock
	ErrSourceExceedsTarget
	ErrHeadOlderThanTarget
	ErrSourceSlotMismatch
	ErrTargetSlotMismatch
	ErrHeadSlotMismatch
	ErrAttestationTooFarInFuture
	ErrAttestationSignatureMismatch
	ErrParticipantsMismatch
	ErrAggregateVerificationFailed
	ErrSignatureAggregationFailed
	ErrMissingTargetState
	ErrNotProposer
	ErrDuplicateAttestationData
	ErrTooManyAttestationData
)

func errMissingParentState(parentRoot [32]byte, slot uint64) error {
	return &StoreError{ErrMissingParentState, fmt.Sprintf("parent state not found for slot %d, missing block %x", slot, parentRoot[:4])}
}

func errUnknownSourceBlock(root [32]byte) error {
	return &StoreError{ErrUnknownSourceBlock, fmt.Sprintf("unknown source block: %x", root[:4])}
}

func errUnknownTargetBlock(root [32]byte) error {
	return &StoreError{ErrUnknownTargetBlock, fmt.Sprintf("unknown target block: %x", root[:4])}
}

func errUnknownHeadBlock(root [32]byte) error {
	return &StoreError{ErrUnknownHeadBlock, fmt.Sprintf("unknown head block: %x", root[:4])}
}

func errSourceExceedsTarget() error {
	return &StoreError{ErrSourceExceedsTarget, "source checkpoint slot exceeds target"}
}

func errHeadOlderThanTarget(headSlot, targetSlot uint64) error {
	return &StoreError{ErrHeadOlderThanTarget, fmt.Sprintf("head slot %d older than target slot %d", headSlot, targetSlot)}
}

func errSourceSlotMismatch(cpSlot, blockSlot uint64) error {
	return &StoreError{ErrSourceSlotMismatch, fmt.Sprintf("source checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errTargetSlotMismatch(cpSlot, blockSlot uint64) error {
	return &StoreError{ErrTargetSlotMismatch, fmt.Sprintf("target checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errHeadSlotMismatch(cpSlot, blockSlot uint64) error {
	return &StoreError{ErrHeadSlotMismatch, fmt.Sprintf("head checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errAttestationTooFarInFuture(attSlot, currentSlot uint64) error {
	return &StoreError{ErrAttestationTooFarInFuture, fmt.Sprintf("attestation slot %d too far in future (current %d)", attSlot, currentSlot)}
}

func errNotProposer(vid, slot uint64) error {
	return &StoreError{ErrNotProposer, fmt.Sprintf("validator %d not proposer for slot %d", vid, slot)}
}

func errMissingTargetState(root [32]byte) error {
	return &StoreError{ErrMissingTargetState, fmt.Sprintf("missing target state: %x", root[:4])}
}
