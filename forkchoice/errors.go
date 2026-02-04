package forkchoice

import "errors"

// Sentinel errors for fork choice. Callers may use errors.Is to check for them.
var (
	ErrParentNotFound = errors.New("parent not found")
	ErrSourceNotFound = errors.New("source root not found")
	ErrTargetNotFound = errors.New("target root not found")
	ErrSlotMismatch   = errors.New("slot mismatch")
	ErrFutureVote     = errors.New("vote too far in future")
)
