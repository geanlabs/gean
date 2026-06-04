package blockbuilder

import (
	"errors"
	"fmt"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

var ErrJustifiedDivergenceNotClosed = errors.New("justified divergence not closed")
var ErrMalformedInput = errors.New("malformed blockbuilder input")
var ErrMalformedPayload = errors.New("malformed payload")
var ErrPayloadHeadUnknown = errors.New("payload head root unknown")
var ErrPayloadRootMismatch = errors.New("payload root mismatch")
var ErrPayloadVoteInvalid = errors.New("payload vote invalid")

var errExpectedSkip = errors.New("expected builder skip")

func IsExpectedSkip(err error) bool { return errors.Is(err, errExpectedSkip) }

func voteReasonExpected(reason string) bool {
	switch reason {
	case statetransition.VoteReasonSourceNotJustified,
		statetransition.VoteReasonTargetAlreadyJustified,
		statetransition.VoteReasonTargetNotJustifiable:
		return true
	default:
		return false
	}
}

func errJustifiedDivergenceNotClosed(actual, required *types.Checkpoint) error {
	actualSlot, requiredSlot := uint64(0), uint64(0)
	actualRoot, requiredRoot := types.ZeroRoot, types.ZeroRoot
	if actual != nil {
		actualSlot = actual.Slot
		actualRoot = actual.Root
	}
	if required != nil {
		requiredSlot = required.Slot
		requiredRoot = required.Root
	}
	return fmt.Errorf(
		"%w: produced checkpoint slot=%d root=0x%x does not satisfy required checkpoint slot=%d root=0x%x",
		ErrJustifiedDivergenceNotClosed,
		actualSlot,
		actualRoot,
		requiredSlot,
		requiredRoot,
	)
}

func errMalformedHeadState(field string) error {
	return fmt.Errorf("%w: head state %s is nil", ErrMalformedInput, field)
}

func errMalformedInput(reason string) error {
	return fmt.Errorf("%w: %s", ErrMalformedInput, reason)
}

func errMalformedPayload(reason string) error {
	return fmt.Errorf("%w: %s", ErrMalformedPayload, reason)
}

func errPayloadRootMismatch(dataRoot, computed [32]byte) error {
	return fmt.Errorf("%w: data root=0x%x computed=0x%x", ErrPayloadRootMismatch, dataRoot, computed)
}

func errPayloadHeadUnknown(root [32]byte) error {
	return fmt.Errorf("%w: head root=0x%x [%w]", ErrPayloadHeadUnknown, root, errExpectedSkip)
}

func errPayloadVoteInvalid(data *types.AttestationData, reason string) error {
	var err error
	if data == nil || data.Source == nil || data.Target == nil {
		err = fmt.Errorf("%w (%s)", ErrPayloadVoteInvalid, reason)
	} else {
		err = fmt.Errorf("%w (%s): source slot=%d root=0x%x target slot=%d root=0x%x",
			ErrPayloadVoteInvalid, reason,
			data.Source.Slot, data.Source.Root, data.Target.Slot, data.Target.Root)
	}
	if voteReasonExpected(reason) {
		err = fmt.Errorf("%w [%w]", err, errExpectedSkip)
	}
	return err
}
