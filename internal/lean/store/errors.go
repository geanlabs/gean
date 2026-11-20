package store

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
	ErrProposerSignatureDecodingFailed
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
	ErrDuplicateAttestationData
	ErrTooManyAttestationData
	ErrSourceNotAncestorOfTarget
	ErrTargetNotAncestorOfHead
	ErrAttestationSlotBeforeHead
	ErrBlockTooFarInFuture
	ErrHeadNotDescendantOfFinalized
)
