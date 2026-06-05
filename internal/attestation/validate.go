package attestation

import (
	"fmt"
	"math"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func errMalformedAttestationData(field string) error {
	return fmt.Errorf("malformed attestation data: %s is nil", field)
}

func errUnknownSourceBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownSourceBlock, Message: fmt.Sprintf("unknown source block: %x", root[:4])}
}

func errUnknownTargetBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownTargetBlock, Message: fmt.Sprintf("unknown target block: %x", root[:4])}
}

func errUnknownHeadBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownHeadBlock, Message: fmt.Sprintf("unknown head block: %x", root[:4])}
}

func errSourceExceedsTarget() error {
	return &store.StoreError{Kind: store.ErrSourceExceedsTarget, Message: "source checkpoint slot exceeds target"}
}

func errHeadOlderThanTarget(headSlot, targetSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadOlderThanTarget, Message: fmt.Sprintf("head slot %d older than target slot %d", headSlot, targetSlot)}
}

func errSourceSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrSourceSlotMismatch, Message: fmt.Sprintf("source checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errTargetSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrTargetSlotMismatch, Message: fmt.Sprintf("target checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errHeadSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadSlotMismatch, Message: fmt.Sprintf("head checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errAttestationTooFarInFuture(attSlot, storeTime uint64) error {
	return &store.StoreError{Kind: store.ErrAttestationTooFarInFuture, Message: fmt.Sprintf("attestation slot %d too far in future (store time %d intervals)", attSlot, storeTime)}
}

func ValidateAttestationData(s *store.ConsensusStore, data *types.AttestationData) error {
	if err := validateDataShape(data); err != nil {
		return err
	}

	sourceHeader := s.GetBlockHeader(data.Source.Root)
	if sourceHeader == nil {
		return errUnknownSourceBlock(data.Source.Root)
	}
	targetHeader := s.GetBlockHeader(data.Target.Root)
	if targetHeader == nil {
		return errUnknownTargetBlock(data.Target.Root)
	}
	headHeader := s.GetBlockHeader(data.Head.Root)
	if headHeader == nil {
		return errUnknownHeadBlock(data.Head.Root)
	}

	if data.Source.Slot > data.Target.Slot {
		return errSourceExceedsTarget()
	}
	if data.Head.Slot < data.Target.Slot {
		return errHeadOlderThanTarget(data.Head.Slot, data.Target.Slot)
	}
	if sourceHeader.Slot != data.Source.Slot {
		return errSourceSlotMismatch(data.Source.Slot, sourceHeader.Slot)
	}
	if targetHeader.Slot != data.Target.Slot {
		return errTargetSlotMismatch(data.Target.Slot, targetHeader.Slot)
	}
	if headHeader.Slot != data.Head.Slot {
		return errHeadSlotMismatch(data.Head.Slot, headHeader.Slot)
	}
	if data.Slot > math.MaxUint64/types.IntervalsPerSlot ||
		data.Slot*types.IntervalsPerSlot > s.Time()+types.GossipDisparityIntervals {
		return errAttestationTooFarInFuture(data.Slot, s.Time())
	}

	return nil
}

func validateDataShape(data *types.AttestationData) error {
	if data == nil {
		return errMalformedAttestationData("data")
	}
	if data.Source == nil {
		return errMalformedAttestationData("source")
	}
	if data.Target == nil {
		return errMalformedAttestationData("target")
	}
	if data.Head == nil {
		return errMalformedAttestationData("head")
	}
	return nil
}
