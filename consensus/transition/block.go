// block.go contains block header validation/application and block-level transition orchestration.
package transition

import (
	"fmt"

	"github.com/devylongs/gean/types"
)

// ProcessBlockHeader validates and applies a block header to the state.
func ProcessBlockHeader(s *types.State, block *types.Block) (*types.State, error) {
	// Validate slot matches
	if block.Slot != s.Slot {
		return nil, fmt.Errorf("block slot %d != state slot %d", block.Slot, s.Slot)
	}

	// Block must be newer than latest header
	if block.Slot <= s.LatestBlockHeader.Slot {
		return nil, fmt.Errorf("block slot %d <= latest header slot %d", block.Slot, s.LatestBlockHeader.Slot)
	}

	// Validate proposer (round-robin)
	expectedProposer := uint64(block.Slot) % uint64(len(s.Validators))
	if block.ProposerIndex != expectedProposer {
		return nil, fmt.Errorf("invalid proposer %d for slot %d, expected %d", block.ProposerIndex, block.Slot, expectedProposer)
	}

	// Validate parent root
	expectedParent, err := s.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash latest header: %w", err)
	}
	if block.ParentRoot != expectedParent {
		return nil, fmt.Errorf("parent root mismatch")
	}

	newState := Copy(s)

	// First block after genesis: mark genesis as justified and finalized
	if s.LatestBlockHeader.Slot == 0 {
		newState.LatestJustified.Root = block.ParentRoot
		newState.LatestFinalized.Root = block.ParentRoot
	}

	// Append parent root to history
	newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, block.ParentRoot)

	// Track justified slot (genesis slot 0 is always justified)
	parentSlot := int(s.LatestBlockHeader.Slot)
	newState.JustifiedSlots = appendBitAt(newState.JustifiedSlots, parentSlot, s.LatestBlockHeader.Slot == 0)

	// Fill empty slots with zero hashes
	emptySlots := int(block.Slot - s.LatestBlockHeader.Slot - 1)
	for i := 0; i < emptySlots; i++ {
		newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, types.Root{})
		emptySlot := parentSlot + 1 + i
		newState.JustifiedSlots = appendBitAt(newState.JustifiedSlots, emptySlot, false)
	}

	// Create new block header (state_root left empty, filled by next ProcessSlots step)
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash body: %w", err)
	}
	newState.LatestBlockHeader = types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     types.Root{},
		BodyRoot:      bodyRoot,
	}

	return newState, nil
}

// ProcessBlock applies process_block_header then process_attestations.
func ProcessBlock(s *types.State, block *types.Block) (*types.State, error) {
	state, err := ProcessBlockHeader(s, block)
	if err != nil {
		return nil, err
	}
	return ProcessAttestations(state, block.Body.Attestations)
}
