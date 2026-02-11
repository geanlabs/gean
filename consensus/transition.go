// Package consensus implements the state transition function.
//
// Pipeline: process_slots (cache state root inline)
//
//	process_block → process_block_header + process_attestations
package consensus

import (
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// ProcessSlots advances the state through empty slots up to targetSlot.
func ProcessSlots(s *types.State, targetSlot types.Slot) (*types.State, error) {
	if s.Slot >= targetSlot {
		return nil, fmt.Errorf("target slot %d must be greater than current slot %d", targetSlot, s.Slot)
	}

	state := Copy(s)
	for state.Slot < targetSlot {
		// Cache state root into the latest header before advancing the slot.
		// This avoids circular dependency during block construction.
		if state.LatestBlockHeader.StateRoot.IsZero() {
			stateRoot, err := state.HashTreeRoot()
			if err != nil {
				return nil, fmt.Errorf("hash state: %w", err)
			}
			state.LatestBlockHeader.StateRoot = stateRoot
		}
		state.Slot++
	}
	return state, nil
}

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

// ProcessAttestations processes attestations and updates justification/finalization (3SF-mini).
// Each valid attestation individually justifies its target (no supermajority counting).
// Finalization: if slots N and N+1 are both justified, slot N becomes finalized.
// TODO: rewrite to use 2/3 supermajority counting.
func ProcessAttestations(s *types.State, attestations []types.Attestation) (*types.State, error) {
	newState := Copy(s)

	bl := bitfield.Bitlist(newState.JustifiedSlots)
	justifiedSlots := make([]bool, bl.Len())
	for i := uint64(0); i < bl.Len(); i++ {
		justifiedSlots[i] = bl.BitAt(i)
	}

	latestJustified := newState.LatestJustified
	latestFinalized := newState.LatestFinalized
	// Original finalized slot used for gap checks (immutable during this call).
	originalFinalizedSlot := newState.LatestFinalized.Slot

	for _, att := range attestations {
		source := att.Data.Source
		target := att.Data.Target

		sourceSlot := int(source.Slot)
		targetSlot := int(target.Slot)

		// Validate source comes before target
		if source.Slot >= target.Slot {
			continue
		}

		// Check if source is justified
		if sourceSlot >= len(justifiedSlots) {
			continue
		}
		if !justifiedSlots[sourceSlot] {
			continue
		}

		// Justify target if not already justified
		alreadyJustified := targetSlot < len(justifiedSlots) && justifiedSlots[targetSlot]
		if !alreadyJustified {
			for len(justifiedSlots) <= targetSlot {
				justifiedSlots = append(justifiedSlots, false)
			}
			justifiedSlots[targetSlot] = true

			if target.Slot > latestJustified.Slot {
				latestJustified = target
			}
		}

		// Finalize source if there are no justifiable slots between source and target.
		// No justifiable gap means the chain of trust is unbroken.
		noGap := true
		for s := int(source.Slot) + 1; s < int(target.Slot); s++ {
			if types.Slot(s).IsJustifiableAfter(originalFinalizedSlot) {
				noGap = false
				break
			}
		}
		if noGap && source.Slot >= latestFinalized.Slot {
			latestFinalized = source
		}
	}

	// Write justifiedSlots back to bitlist
	newBl := bitfield.NewBitlist(uint64(len(justifiedSlots)))
	for i, v := range justifiedSlots {
		if v {
			newBl.SetBitAt(uint64(i), true)
		}
	}
	newState.JustifiedSlots = newBl
	newState.LatestJustified = latestJustified
	newState.LatestFinalized = latestFinalized

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

// Copy creates a deep copy of the state.
func Copy(s *types.State) *types.State {
	cp := *s
	cp.HistoricalBlockHashes = append([]types.Root{}, s.HistoricalBlockHashes...)
	cp.JustifiedSlots = append([]byte{}, s.JustifiedSlots...)
	cp.Validators = append([]types.Validator{}, s.Validators...)
	cp.JustificationRoots = append([]types.Root{}, s.JustificationRoots...)
	cp.JustificationValidators = append([]byte{}, s.JustificationValidators...)
	return &cp
}

// setBit sets a bit at the given index.
// If the bitlist needs to grow, it creates a new one with sufficient capacity.
func setBit(bits []byte, index int, val bool) []byte {
	bl := bitfield.Bitlist(bits)
	idx := uint64(index)

	// If we need more capacity, create a larger bitlist
	if idx >= bl.Len() {
		newBl := bitfield.NewBitlist(idx + 1)
		// Copy existing bits
		for i := uint64(0); i < bl.Len(); i++ {
			if bl.BitAt(i) {
				newBl.SetBitAt(i, true)
			}
		}
		bl = newBl
	}

	bl.SetBitAt(idx, val)
	return bl
}

// appendBitAt appends a bit at the given index, extending the bitlist if needed.
func appendBitAt(bits []byte, index int, val bool) []byte {
	if len(bits) == 0 {
		bits = bitfield.NewBitlist(uint64(index) + 1)
	}
	return setBit(bits, index, val)
}
