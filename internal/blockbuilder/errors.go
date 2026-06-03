package blockbuilder

import (
	"errors"
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

var ErrJustifiedDivergenceNotClosed = errors.New("justified divergence not closed")
var ErrMalformedInput = errors.New("malformed blockbuilder input")
var ErrMalformedPayload = errors.New("malformed payload")
var ErrPayloadHeadUnknown = errors.New("payload head root unknown")
var ErrPayloadRootMismatch = errors.New("payload root mismatch")
var ErrPayloadVoteInvalid = errors.New("payload vote invalid")

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
	return fmt.Errorf("%w: head root=0x%x", ErrPayloadHeadUnknown, root)
}

func errPayloadVoteInvalid(data *types.AttestationData) error {
	if data == nil || data.Source == nil || data.Target == nil {
		return ErrPayloadVoteInvalid
	}
	return fmt.Errorf("%w: source slot=%d root=0x%x target slot=%d root=0x%x",
		ErrPayloadVoteInvalid,
		data.Source.Slot,
		data.Source.Root,
		data.Target.Slot,
		data.Target.Root,
	)
}
