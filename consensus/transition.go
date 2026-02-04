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
// Per leanSpec chain.md process_attestations:
// 1. Get justifications map from state
// 2. For each vote, validate and track per-validator votes
// 3. Justify when 2/3 supermajority (3*count >= 2*num_validators)
// 4. Finalize when no intermediate justifiable slots between source and target
// 5. Persist justifications back to state
func ProcessAttestations(s *types.State, attestations []types.SignedVote) (*types.State, error) {
	newState := Copy(s)

	// Get the justifications map from flattened state
	justifications := GetJustifications(newState)

	for _, signedVote := range attestations {
		vote := signedVote.Message

		sourceSlot := int(vote.Source.Slot)
		targetSlot := int(vote.Target.Slot)
		validatorID := int(signedVote.ValidatorID)

		// Validation 1: Source must be justified
		if !getBit(newState.JustifiedSlots, sourceSlot) {
			continue
		}

		// Validation 2: Target must NOT already be justified
		// (per spec: we don't want to re-introduce the target for remaining votes
		// if the slot is already justified and its tracking already cleared)
		if getBit(newState.JustifiedSlots, targetSlot) {
			continue
		}

		// Validation 3: Source root must match historical block hash at that slot
		if sourceSlot >= len(newState.HistoricalBlockHashes) ||
			vote.Source.Root != newState.HistoricalBlockHashes[sourceSlot] {
			continue
		}

		// Validation 4: Target root must match historical block hash at that slot
		if targetSlot >= len(newState.HistoricalBlockHashes) ||
			vote.Target.Root != newState.HistoricalBlockHashes[targetSlot] {
			continue
		}

		// Validation 5: Target slot must be greater than source slot
		if vote.Target.Slot <= vote.Source.Slot {
			continue
		}

		// Validation 6: Target must be a justifiable slot after finalized
		if !vote.Target.Slot.IsJustifiableAfter(newState.LatestFinalized.Slot) {
			continue
		}

		// Track the vote in justifications map
		if _, exists := justifications[vote.Target.Root]; !exists {
			justifications[vote.Target.Root] = make([]bool, newState.Config.NumValidators)
		}

		// Only count if validator hasn't already voted for this target
		if !justifications[vote.Target.Root][validatorID] {
			justifications[vote.Target.Root][validatorID] = true
		}

		// Count votes for this target
		count := CountVotes(justifications[vote.Target.Root])

		// Check for 2/3 supermajority: 3*count >= 2*num_validators
		// (spec uses this form to avoid integer division issues)
		if 3*count >= 2*int(newState.Config.NumValidators) {
			// Justify the target
			newState.LatestJustified = vote.Target
			newState.JustifiedSlots = setBit(newState.JustifiedSlots, targetSlot, true)

			// Remove from justifications map (no longer under voting consideration)
			delete(justifications, vote.Target.Root)

			// Finalization check: target is the next valid justifiable slot after source
			// (no intermediate justifiable slots between source and target)
			canFinalize := true
			for slot := vote.Source.Slot + 1; slot < vote.Target.Slot; slot++ {
				if slot.IsJustifiableAfter(newState.LatestFinalized.Slot) {
					canFinalize = false
					break
				}
			}

			if canFinalize {
				newState.LatestFinalized = vote.Source
			}
		}
	}

	// Persist the justifications map back to state
	newState = SetJustifications(newState, justifications)

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
