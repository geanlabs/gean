// Package chain implements the Lean Ethereum state transition function.
package chain

import (
	"fmt"

	"github.com/devylongs/gean/types"
)

var ZeroHash = types.Root{}

// ProcessSlot performs per-slot maintenance.
// If the latest block header has an empty state_root, fill it with the current state root.
func ProcessSlot(s *types.State) (*types.State, error) {
	if s.LatestBlockHeader.StateRoot.IsZero() {
		stateRoot, err := s.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("hash state: %w", err)
		}
		newState := Copy(s)
		newState.LatestBlockHeader.StateRoot = stateRoot
		return newState, nil
	}
	return s, nil
}

// ProcessSlots advances the state through empty slots up to targetSlot.
func ProcessSlots(s *types.State, targetSlot types.Slot) (*types.State, error) {
	if s.Slot >= targetSlot {
		return nil, fmt.Errorf("target slot %d must be greater than current slot %d", targetSlot, s.Slot)
	}

	state := s
	var err error
	for state.Slot < targetSlot {
		state, err = ProcessSlot(state)
		if err != nil {
			return nil, err
		}
		newState := Copy(state)
		newState.Slot++
		state = newState
	}
	return state, nil
}

// ProcessBlockHeader validates and applies a block header.
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
	expectedProposer := uint64(block.Slot) % s.Config.NumValidators
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
		newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, ZeroHash)
		emptySlot := parentSlot + 1 + i
		newState.JustifiedSlots = appendBitAt(newState.JustifiedSlots, emptySlot, false)
	}

	// Create new block header (state_root left empty, filled by next ProcessSlot)
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

// ProcessAttestations processes attestation votes per Devnet 0 spec.
// Per the spec, justification happens when source is justified and we vote for a target.
// Finalization happens when source and target are consecutive justified slots.
func ProcessAttestations(s *types.State, attestations []types.SignedVote) (*types.State, error) {
	newState := Copy(s)

	for _, signed := range attestations {
		vote := signed.Data

		// Skip if source slot >= target slot
		if vote.Source.Slot >= vote.Target.Slot {
			continue
		}

		sourceSlot := int(vote.Source.Slot)
		targetSlot := int(vote.Target.Slot)

		// Skip if source is not justified
		if !getBit(newState.JustifiedSlots, sourceSlot) {
			continue
		}

		// Check if target is already justified
		if getBit(newState.JustifiedSlots, targetSlot) {
			// Target already justified - check for finalization
			// Finalization occurs when source and target are consecutive justified checkpoints
			if vote.Source.Slot+1 == vote.Target.Slot &&
				newState.LatestJustified.Slot < vote.Target.Slot {
				newState.LatestFinalized = vote.Source
				newState.LatestJustified = vote.Target
			}
			continue
		}

		// Extend JustifiedSlots if needed
		for targetSlot/8 >= len(newState.JustifiedSlots) {
			newState.JustifiedSlots = append(newState.JustifiedSlots, 0)
		}

		// Justify the target
		newState.JustifiedSlots = setBit(newState.JustifiedSlots, targetSlot, true)

		// Update latest justified if this target is newer
		if vote.Target.Slot > newState.LatestJustified.Slot {
			newState.LatestJustified = vote.Target
		}
	}

	return newState, nil
}

// ProcessBlock applies full block processing.
func ProcessBlock(s *types.State, block *types.Block) (*types.State, error) {
	state, err := ProcessBlockHeader(s, block)
	if err != nil {
		return nil, err
	}
	return ProcessAttestations(state, block.Body.Attestations)
}

// StateTransition applies the complete state transition for a signed block.
// For Devnet 0, valid_signatures is always true (no signature verification).
func StateTransition(s *types.State, signedBlock *types.SignedBlock, validateResult bool) (*types.State, error) {
	block := &signedBlock.Message

	// Process slots up to block slot
	state, err := ProcessSlots(s, block.Slot)
	if err != nil {
		return nil, err
	}

	// Process the block
	newState, err := ProcessBlock(state, block)
	if err != nil {
		return nil, err
	}

	// Validate state root
	if validateResult {
		computedRoot, err := newState.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("hash new state: %w", err)
		}
		if block.StateRoot != computedRoot {
			return nil, fmt.Errorf("state root mismatch: expected %x, got %x", block.StateRoot, computedRoot)
		}
	}

	return newState, nil
}

// Copy creates a deep copy of the state.
func Copy(s *types.State) *types.State {
	cp := *s
	cp.HistoricalBlockHashes = append([]types.Root{}, s.HistoricalBlockHashes...)
	cp.JustifiedSlots = append([]byte{}, s.JustifiedSlots...)
	cp.JustificationRoots = append([]types.Root{}, s.JustificationRoots...)
	cp.JustificationValidators = append([]byte{}, s.JustificationValidators...)
	return &cp
}

// Bitlist helpers

// appendBitAt sets a bit at the given index, extending the slice if needed.
func appendBitAt(bits []byte, index int, val bool) []byte {
	byteIndex := index / 8
	for byteIndex >= len(bits) {
		bits = append(bits, 0)
	}
	if val {
		bits[byteIndex] |= 1 << (index % 8)
	}
	return bits
}

func getBit(bits []byte, index int) bool {
	if index/8 >= len(bits) {
		return false
	}
	return bits[index/8]&(1<<(index%8)) != 0
}

func setBit(bits []byte, index int, val bool) []byte {
	for index/8 >= len(bits) {
		bits = append(bits, 0)
	}
	if val {
		bits[index/8] |= 1 << (index % 8)
	} else {
		bits[index/8] &^= 1 << (index % 8)
	}
	return bits
}
