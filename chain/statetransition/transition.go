package statetransition

import (
	"fmt"

	"github.com/geanlabs/gean/types"
)

// ProcessSlot performs per-slot maintenance. If the latest block header has
// a zero state_root, it caches the current state root into that header.
func ProcessSlot(state *types.State) *types.State {
	if state.LatestBlockHeader.StateRoot == types.ZeroHash {
		stateRoot, _ := state.HashTreeRoot()
		out := copyState(state)
		out.LatestBlockHeader = copyHeader(state.LatestBlockHeader)
		out.LatestBlockHeader.StateRoot = stateRoot
		return out
	}
	return state
}

// ProcessSlots advances the state through empty slots up to targetSlot.
func ProcessSlots(state *types.State, targetSlot uint64) (*types.State, error) {
	if state.Slot >= targetSlot {
		return nil, fmt.Errorf("target slot %d must be after current slot %d", targetSlot, state.Slot)
	}
	s := state
	for s.Slot < targetSlot {
		s = ProcessSlot(s)
		out := copyState(s)
		out.Slot = s.Slot + 1
		s = out
	}
	return s, nil
}

// ProcessBlockHeader validates the block header and updates header-linked state.
func ProcessBlockHeader(state *types.State, block *types.Block) (*types.State, error) {
	if block.Slot != state.Slot {
		return nil, fmt.Errorf("block slot %d != state slot %d", block.Slot, state.Slot)
	}
	if block.Slot <= state.LatestBlockHeader.Slot {
		return nil, fmt.Errorf("block slot %d <= latest header slot %d", block.Slot, state.LatestBlockHeader.Slot)
	}
	if !IsProposer(block.ProposerIndex, state.Slot, state.Config.NumValidators) {
		return nil, fmt.Errorf("validator %d is not proposer for slot %d", block.ProposerIndex, state.Slot)
	}

	expectedParent, _ := state.LatestBlockHeader.HashTreeRoot()
	if block.ParentRoot != expectedParent {
		return nil, fmt.Errorf("parent root mismatch")
	}

	out := copyState(state)
	parentRoot := block.ParentRoot

	// First block after genesis: mark genesis as justified and finalized.
	if state.LatestBlockHeader.Slot == 0 {
		out.LatestJustified = &types.Checkpoint{Root: parentRoot, Slot: state.LatestJustified.Slot}
		out.LatestFinalized = &types.Checkpoint{Root: parentRoot, Slot: state.LatestFinalized.Slot}
	}

	// Append parent root to historical hashes.
	out.HistoricalBlockHashes = append(cloneHashes(state.HistoricalBlockHashes), parentRoot)

	// Append justified bit for parent: true only for genesis slot.
	out.JustifiedSlots = appendBit(cloneBitlist(state.JustifiedSlots), state.LatestBlockHeader.Slot == 0)

	// Fill empty slots between parent and this block.
	numEmpty := block.Slot - state.LatestBlockHeader.Slot - 1
	for i := uint64(0); i < numEmpty; i++ {
		out.HistoricalBlockHashes = append(out.HistoricalBlockHashes, types.ZeroHash)
		out.JustifiedSlots = appendBit(out.JustifiedSlots, false)
	}

	// Build new latest block header with zero state_root (filled on next process_slot).
	bodyRoot, _ := block.Body.HashTreeRoot()
	out.LatestBlockHeader = &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		BodyRoot:      bodyRoot,
		StateRoot:     types.ZeroHash,
	}

	return out, nil
}

// ProcessBlock applies full block processing: header + attestations.
func ProcessBlock(state *types.State, block *types.Block) (*types.State, error) {
	s, err := ProcessBlockHeader(state, block)
	if err != nil {
		return nil, err
	}
	s = ProcessAttestations(s, block.Body.Attestations)
	return s, nil
}

// StateTransition applies the complete state transition for a signed block.
func StateTransition(state *types.State, signedBlock *types.SignedBlock) (*types.State, error) {
	block := signedBlock.Message

	// Process intermediate slots.
	s, err := ProcessSlots(state, block.Slot)
	if err != nil {
		return nil, fmt.Errorf("process_slots: %w", err)
	}

	// Process the block.
	s, err = ProcessBlock(s, block)
	if err != nil {
		return nil, fmt.Errorf("process_block: %w", err)
	}

	// Validate state root.
	computedRoot, _ := s.HashTreeRoot()
	if block.StateRoot != computedRoot {
		return nil, fmt.Errorf("invalid state root: expected %x, got %x", computedRoot, block.StateRoot)
	}

	return s, nil
}

// --- helpers ---

func copyState(s *types.State) *types.State {
	return &types.State{
		Config:                   s.Config,
		Slot:                     s.Slot,
		LatestBlockHeader:        s.LatestBlockHeader,
		LatestJustified:          s.LatestJustified,
		LatestFinalized:          s.LatestFinalized,
		HistoricalBlockHashes:    s.HistoricalBlockHashes,
		JustifiedSlots:           s.JustifiedSlots,
		JustificationsRoots:      s.JustificationsRoots,
		JustificationsValidators: s.JustificationsValidators,
	}
}

func copyHeader(h *types.BlockHeader) *types.BlockHeader {
	return &types.BlockHeader{
		Slot:          h.Slot,
		ProposerIndex: h.ProposerIndex,
		ParentRoot:    h.ParentRoot,
		StateRoot:     h.StateRoot,
		BodyRoot:      h.BodyRoot,
	}
}

func cloneHashes(src [][32]byte) [][32]byte {
	dst := make([][32]byte, len(src))
	copy(dst, src)
	return dst
}
