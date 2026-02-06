// Package consensus implements the Lean Ethereum state transition function.
package consensus

import (
	"fmt"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

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
		newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, types.Root{})
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

// ProcessAttestations processes attestation votes per leanSpec Devnet 0.
//
// Per leanSpec state.py process_attestations:
// 1. For each vote, validate source < target
// 2. If source is justified and target already justified: check finalization
// 3. If source is justified and target not justified: justify target
// 4. No supermajority counting — each valid attestation individually justifies
func ProcessAttestations(s *types.State, attestations []types.SignedVote) (*types.State, error) {
	newState := Copy(s)

	bl := bitfield.Bitlist(newState.JustifiedSlots)
	justifiedSlots := make([]bool, bl.Len())
	for i := uint64(0); i < bl.Len(); i++ {
		justifiedSlots[i] = bl.BitAt(i)
	}

	latestJustified := newState.LatestJustified
	latestFinalized := newState.LatestFinalized

	for _, signedVote := range attestations {
		vote := signedVote.Data
		source := vote.Source
		target := vote.Target

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
		sourceIsJustified := justifiedSlots[sourceSlot]

		if sourceIsJustified && targetSlot < len(justifiedSlots) && justifiedSlots[targetSlot] {
			// Target already justified — check for finalization
			// Consecutive justified slots finalize the source
			if int(source.Slot)+1 == int(target.Slot) &&
				int(latestJustified.Slot) < int(target.Slot) {
				latestFinalized = source
				latestJustified = target
			}
		} else if sourceIsJustified {
			// Source is justified, target is not — justify the target
			for len(justifiedSlots) <= targetSlot {
				justifiedSlots = append(justifiedSlots, false)
			}
			justifiedSlots[targetSlot] = true

			// Update latest_justified if target is newer
			if target.Slot > latestJustified.Slot {
				latestJustified = target
			}
		}
	}

	// Write justifiedSlots back to bitlist
	maxIdx := len(justifiedSlots)
	if maxIdx == 0 {
		maxIdx = 1
	}
	newBl := bitfield.NewBitlist(uint64(maxIdx))
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

// ProcessBlock applies full block processing.
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
	cp.JustificationRoots = append([]types.Root{}, s.JustificationRoots...)
	cp.JustificationValidators = append([]byte{}, s.JustificationValidators...)
	return &cp
}

// getBit returns the value of a bit at the given index.
// Returns false if index is out of bounds.
func getBit(bits []byte, index int) bool {
	bl := bitfield.Bitlist(bits)
	if uint64(index) >= bl.Len() {
		return false
	}
	return bl.BitAt(uint64(index))
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
