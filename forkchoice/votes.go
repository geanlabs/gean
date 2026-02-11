package forkchoice

import (
	"fmt"

	"github.com/devylongs/gean/types"
)

// ValidateAttestation validates an attestation against the current store state.
func (s *Store) ValidateAttestation(signed *types.SignedAttestation) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateAttestationLocked(signed)
}

// validateAttestationLocked validates an attestation's structural correctness.
// Note: the spec checks future votes using intervals; we use slots (+1 tolerance).
func (s *Store) validateAttestationLocked(signed *types.SignedAttestation) error {
	validatorID := signed.Message.ValidatorID
	data := signed.Message.Data

	// Validate validator index is in range.
	if validatorID >= uint64(len(s.LatestKnownVotes)) {
		return fmt.Errorf("%w: validator_id %d >= %d",
			ErrValidatorOutOfRange, validatorID, len(s.LatestKnownVotes))
	}

	// Validate head exists in store.
	if _, exists := s.Blocks[data.Head.Root]; !exists {
		return fmt.Errorf("%w: head root %x", ErrHeadNotFound, data.Head.Root[:8])
	}

	// Validate target exists in store
	if _, exists := s.Blocks[data.Target.Root]; !exists {
		return fmt.Errorf("%w: target root %x", ErrTargetNotFound, data.Target.Root[:8])
	}
	targetBlock := s.Blocks[data.Target.Root]

	// Validate source exists (zero root is valid for genesis checkpoint)
	var sourceSlot types.Slot
	if data.Source.Root.IsZero() {
		// Genesis checkpoint - source slot must be 0
		if data.Source.Slot != 0 {
			return fmt.Errorf("%w: genesis source must have slot 0, got %d",
				ErrSlotMismatch, data.Source.Slot)
		}
		sourceSlot = 0
	} else {
		sourceBlock, exists := s.Blocks[data.Source.Root]
		if !exists {
			return fmt.Errorf("%w: source root %x", ErrSourceNotFound, data.Source.Root[:8])
		}
		sourceSlot = sourceBlock.Slot

		// Validate checkpoint slot matches block slot
		if sourceSlot != data.Source.Slot {
			return fmt.Errorf("%w: source block slot %d != checkpoint slot %d",
				ErrSlotMismatch, sourceSlot, data.Source.Slot)
		}
	}

	// Validate slot relationships
	if sourceSlot > targetBlock.Slot {
		return fmt.Errorf("%w: source slot %d > target block slot %d",
			ErrSlotMismatch, sourceSlot, targetBlock.Slot)
	}
	if data.Source.Slot > data.Target.Slot {
		return fmt.Errorf("%w: source slot %d > target slot %d",
			ErrSlotMismatch, data.Source.Slot, data.Target.Slot)
	}
	if targetBlock.Slot != data.Target.Slot {
		return fmt.Errorf("%w: target block slot %d != checkpoint slot %d",
			ErrSlotMismatch, targetBlock.Slot, data.Target.Slot)
	}

	// Validate attestation is not too far in future
	currentSlot := s.Clock.CurrentSlot()
	if data.Slot > currentSlot+1 {
		return fmt.Errorf("%w: attestation slot %d too far ahead (current: %d)",
			ErrFutureVote, data.Slot, currentSlot)
	}

	return nil
}

// ProcessAttestation validates and processes a gossipsub attestation as a "new" vote.
func (s *Store) ProcessAttestation(signed *types.SignedAttestation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateAttestationLocked(signed); err != nil {
		return err
	}
	s.processAttestationLocked(signed, false)
	return nil
}

// processAttestationLocked stores an attestation. Block attestations go to LatestKnownVotes
// directly; gossip attestations go to LatestNewVotes (promoted later by acceptNewVotes).
func (s *Store) processAttestationLocked(signed *types.SignedAttestation, isFromBlock bool) {
	att := signed.Message
	idx := att.ValidatorID
	if idx >= uint64(len(s.LatestKnownVotes)) {
		// Defensive guard: invalid validator indices must never panic the store.
		return
	}
	i := int(idx)

	if isFromBlock {
		known := s.LatestKnownVotes[i]
		if known.Root.IsZero() || known.Slot < att.Data.Slot {
			s.LatestKnownVotes[i] = att.Data.Target
		}
		newVote := s.LatestNewVotes[i]
		if !newVote.Root.IsZero() && newVote.Slot <= att.Data.Target.Slot {
			s.LatestNewVotes[i] = types.Checkpoint{}
		}
	} else {
		newVote := s.LatestNewVotes[i]
		if newVote.Root.IsZero() || newVote.Slot < att.Data.Target.Slot {
			s.LatestNewVotes[i] = att.Data.Target
		}
	}
}

// acceptNewVotesLocked promotes pending new votes to known and recalculates head.
func (s *Store) acceptNewVotesLocked() {
	for i, vote := range s.LatestNewVotes {
		if !vote.Root.IsZero() {
			s.LatestKnownVotes[i] = vote
			s.LatestNewVotes[i] = types.Checkpoint{}
		}
	}
	s.updateHeadLocked()
}

// getVoteTargetLocked walks back from head to find a safe, justifiable attestation target.
// Walks back up to 3 steps toward safe target, then further to a justifiable slot.
func (s *Store) getVoteTargetLocked() types.Checkpoint {
	targetRoot := s.Head

	// Walk back up to 3 steps if safe target is newer
	for i := 0; i < 3; i++ {
		if s.Blocks[targetRoot].Slot > s.Blocks[s.SafeTarget].Slot {
			targetRoot = s.Blocks[targetRoot].ParentRoot
		}
	}

	// Ensure target is in justifiable slot range
	for !s.Blocks[targetRoot].Slot.IsJustifiableAfter(s.LatestFinalized.Slot) {
		targetRoot = s.Blocks[targetRoot].ParentRoot
	}

	block := s.Blocks[targetRoot]
	blockRoot, _ := block.HashTreeRoot()
	return types.Checkpoint{Root: blockRoot, Slot: block.Slot}
}
