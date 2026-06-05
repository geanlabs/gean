package statetransition

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func ProcessBlock(state *types.State, block *types.Block) error {
	if err := ProcessBlockHeader(state, block); err != nil {
		return err
	}
	return ProcessAttestations(state, block.Body.Attestations)
}

func ProcessBlockHeader(state *types.State, block *types.Block) error {
	if state == nil {
		return malformedState("state")
	}
	if state.LatestBlockHeader == nil {
		return malformedState("latest block header")
	}
	if state.LatestJustified == nil {
		return malformedState("latest justified")
	}
	if state.LatestFinalized == nil {
		return malformedState("latest finalized")
	}
	if block == nil {
		return malformedBlock("block")
	}
	if block.Body == nil {
		return malformedBlock("body")
	}
	if err := validateBlockBody(block.Body); err != nil {
		return err
	}

	numValidators := state.NumValidators()
	if numValidators == 0 {
		return ErrNoValidators
	}

	if block.Slot != state.Slot {
		return &SlotMismatchError{StateSlot: state.Slot, BlockSlot: block.Slot}
	}

	parentHeader := state.LatestBlockHeader
	if block.Slot <= parentHeader.Slot {
		return &ParentSlotIsNewerError{ParentSlot: parentHeader.Slot, BlockSlot: block.Slot}
	}

	expectedProposer := types.ProposerIndex(block.Slot, numValidators)
	if block.ProposerIndex != expectedProposer {
		return &InvalidProposerError{Expected: expectedProposer, Found: block.ProposerIndex}
	}

	parentRoot, err := parentHeader.HashTreeRoot()
	if err != nil {
		return err
	}
	if block.ParentRoot != parentRoot {
		return &InvalidParentError{Expected: parentRoot, Found: block.ParentRoot}
	}

	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return err
	}

	numEmptySlots := block.Slot - parentHeader.Slot - 1
	newEntries := 1 + numEmptySlots
	currentHistoricalRoots := uint64(len(state.HistoricalBlockHashes))
	if currentHistoricalRoots > types.HistoricalRootsLimit ||
		newEntries > types.HistoricalRootsLimit-currentHistoricalRoots {
		return &SlotGapTooLargeError{
			Gap:     newEntries,
			Current: state.Slot,
			Max:     types.HistoricalRootsLimit,
		}
	}

	if parentHeader.Slot == 0 {
		state.LatestJustified.Root = parentRoot
		state.LatestFinalized.Root = parentRoot
	}

	state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, copyRootBytes(parentRoot))
	for range numEmptySlots {
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, make([]byte, types.RootSize))
	}

	lastMaterializedSlot := block.Slot - 1
	if lastMaterializedSlot > state.LatestFinalized.Slot {
		requiredLen := lastMaterializedSlot - state.LatestFinalized.Slot
		currentLen := types.BitlistLen(state.JustifiedSlots)
		if requiredLen > currentLen {
			state.JustifiedSlots = types.BitlistExtend(state.JustifiedSlots, requiredLen)
		}
	}

	state.LatestBlockHeader = &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     types.ZeroRoot,
		BodyRoot:      bodyRoot,
	}

	return nil
}

func validateBlockBody(body *types.BlockBody) error {
	for i, attestation := range body.Attestations {
		if attestation == nil {
			return malformedBlock(fmt.Sprintf("body.attestations[%d]", i))
		}
	}
	return nil
}
