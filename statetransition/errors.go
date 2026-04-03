package statetransition

import "fmt"

type StateSlotIsNewerError struct {
	TargetSlot  uint64
	CurrentSlot uint64
}

func (e *StateSlotIsNewerError) Error() string {
	return fmt.Sprintf("state slot %d >= target slot %d", e.CurrentSlot, e.TargetSlot)
}

type SlotMismatchError struct {
	StateSlot uint64
	BlockSlot uint64
}

func (e *SlotMismatchError) Error() string {
	return fmt.Sprintf("state slot %d != block slot %d", e.StateSlot, e.BlockSlot)
}

type ParentSlotIsNewerError struct {
	ParentSlot uint64
	BlockSlot  uint64
}

func (e *ParentSlotIsNewerError) Error() string {
	return fmt.Sprintf("parent slot %d >= block slot %d", e.ParentSlot, e.BlockSlot)
}

type InvalidProposerError struct {
	Expected uint64
	Found    uint64
}

func (e *InvalidProposerError) Error() string {
	return fmt.Sprintf("invalid proposer: expected %d, got %d", e.Expected, e.Found)
}

type InvalidParentError struct {
	Expected [32]byte
	Found    [32]byte
}

func (e *InvalidParentError) Error() string {
	return fmt.Sprintf("invalid parent root: expected %x, got %x", e.Expected[:4], e.Found[:4])
}

type StateRootMismatchError struct {
	Expected [32]byte
	Computed [32]byte
}

func (e *StateRootMismatchError) Error() string {
	return fmt.Sprintf("state root mismatch: block has %x, computed %x", e.Expected[:4], e.Computed[:4])
}

type SlotGapTooLargeError struct {
	Gap     uint64
	Current uint64
	Max     uint64
}

func (e *SlotGapTooLargeError) Error() string {
	return fmt.Sprintf("slot gap %d at slot %d exceeds max %d", e.Gap, e.Current, e.Max)
}

var ErrNoValidators = fmt.Errorf("state has no validators")
var ErrZeroHashInJustificationRoots = fmt.Errorf("zero hash found in justifications_roots")
