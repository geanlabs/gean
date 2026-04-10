package statetransition

import (
	"github.com/geanlabs/gean/types"
)

// ProcessBlock validates a block and applies it to the state.
func ProcessBlock(state *types.State, block *types.Block) error {
	if err := ProcessBlockHeader(state, block); err != nil {
		return err
	}
	return ProcessAttestations(state, block.Body.Attestations)
}

// ProcessBlockHeader validates the block header and updates the state.
func ProcessBlockHeader(state *types.State, block *types.Block) error {
	numValidators := state.NumValidators()
	if numValidators == 0 {
		return ErrNoValidators
	}

	// Block slot must match state slot (after process_slots).
	if block.Slot != state.Slot {
		return &SlotMismatchError{StateSlot: state.Slot, BlockSlot: block.Slot}
	}

	// Block must be newer than parent.
	parentHeader := state.LatestBlockHeader
	if block.Slot <= parentHeader.Slot {
		return &ParentSlotIsNewerError{ParentSlot: parentHeader.Slot, BlockSlot: block.Slot}
	}

	// Proposer must be correct: slot % num_validators.
	expectedProposer := block.Slot % numValidators
	if block.ProposerIndex != expectedProposer {
		return &InvalidProposerError{Expected: expectedProposer, Found: block.ProposerIndex}
	}

	// Parent root must match hash of latest block header.
	parentRoot, err := parentHeader.HashTreeRoot()
	if err != nil {
		return err
	}
	if block.ParentRoot != parentRoot {
		return &InvalidParentError{Expected: parentRoot, Found: block.ParentRoot}
	}

	// Genesis parent special case: initialize justified/finalized checkpoints.
	if parentHeader.Slot == 0 {
		state.LatestJustified.Root = parentRoot
		state.LatestFinalized.Root = parentRoot
	}

	// Guard against overflowing historical_block_hashes.
	numEmptySlots := block.Slot - parentHeader.Slot - 1
	newEntries := 1 + numEmptySlots
	if uint64(len(state.HistoricalBlockHashes))+newEntries > types.HistoricalRootsLimit {
		return &SlotGapTooLargeError{
			Gap:     newEntries,
			Current: state.Slot,
			Max:     types.HistoricalRootsLimit,
		}
	}

	// Append parent root + zeros for skipped slots to historical_block_hashes.
	parentRootBytes := parentRoot[:]
	state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, parentRootBytes)
	for i := uint64(0); i < numEmptySlots; i++ {
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, make([]byte, 32))
	}

	// Extend justified_slots to cover slots up to block.slot - 1, relative to finalized boundary.

	lastMaterializedSlot := block.Slot - 1
	if lastMaterializedSlot > state.LatestFinalized.Slot {
		requiredLen := lastMaterializedSlot - state.LatestFinalized.Slot
		currentLen := types.BitlistLen(state.JustifiedSlots)
		if requiredLen > currentLen {
			state.JustifiedSlots = types.BitlistExtend(state.JustifiedSlots, requiredLen)
		}
	}

	// Compute body root for the new block header.
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return err
	}

	// Update latest block header (state_root intentionally zeroed; filled in process_slots).
	state.LatestBlockHeader = &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     types.ZeroRoot,
		BodyRoot:      bodyRoot,
	}

	return nil
}
