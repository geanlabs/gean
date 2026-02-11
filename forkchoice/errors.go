package forkchoice

import "errors"

// Sentinel errors for fork choice validation.
// Callers may use errors.Is to check for specific failure types.
var (
	ErrParentNotFound      = errors.New("parent not found")             // block's parent root not in store
	ErrSourceNotFound      = errors.New("source root not found")        // attestation source root not in store
	ErrTargetNotFound      = errors.New("target root not found")        // attestation target root not in store
	ErrHeadNotFound        = errors.New("head root not found")          // attestation head root not in store
	ErrValidatorOutOfRange = errors.New("validator index out of range") // attestation validator index >= validator count
	ErrSlotMismatch        = errors.New("slot mismatch")                // checkpoint slot doesn't match block slot
	ErrFutureVote          = errors.New("vote too far in future")       // vote.Slot > currentSlot + 1
)
