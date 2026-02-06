package forkchoice

import (
	"fmt"

	"github.com/devylongs/gean/types"
)

// ValidateAttestation validates an attestation according to Devnet 0 spec.
func (s *Store) ValidateAttestation(signedVote *types.SignedVote) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateAttestationLocked(signedVote)
}

func (s *Store) validateAttestationLocked(signedVote *types.SignedVote) error {
	vote := signedVote.Data

	// Validate target exists in store
	if _, exists := s.Blocks[vote.Target.Root]; !exists {
		return fmt.Errorf("%w: target root %x", ErrTargetNotFound, vote.Target.Root[:8])
	}
	targetBlock := s.Blocks[vote.Target.Root]

	// Validate source exists (zero root is valid for genesis checkpoint)
	var sourceSlot types.Slot
	if vote.Source.Root.IsZero() {
		// Genesis checkpoint - source slot must be 0
		if vote.Source.Slot != 0 {
			return fmt.Errorf("%w: genesis source must have slot 0, got %d",
				ErrSlotMismatch, vote.Source.Slot)
		}
		sourceSlot = 0
	} else {
		sourceBlock, exists := s.Blocks[vote.Source.Root]
		if !exists {
			return fmt.Errorf("%w: source root %x", ErrSourceNotFound, vote.Source.Root[:8])
		}
		sourceSlot = sourceBlock.Slot

		// Validate checkpoint slot matches block slot
		if sourceSlot != vote.Source.Slot {
			return fmt.Errorf("%w: source block slot %d != checkpoint slot %d",
				ErrSlotMismatch, sourceSlot, vote.Source.Slot)
		}
	}

	// Validate slot relationships
	if sourceSlot > targetBlock.Slot {
		return fmt.Errorf("%w: source slot %d > target block slot %d",
			ErrSlotMismatch, sourceSlot, targetBlock.Slot)
	}
	if vote.Source.Slot > vote.Target.Slot {
		return fmt.Errorf("%w: source slot %d > target slot %d",
			ErrSlotMismatch, vote.Source.Slot, vote.Target.Slot)
	}
	if targetBlock.Slot != vote.Target.Slot {
		return fmt.Errorf("%w: target block slot %d != checkpoint slot %d",
			ErrSlotMismatch, targetBlock.Slot, vote.Target.Slot)
	}

	// Validate attestation is not too far in future
	currentSlot := s.Clock.CurrentSlot()
	if vote.Slot > currentSlot+1 {
		return fmt.Errorf("%w: vote slot %d too far ahead (current: %d)",
			ErrFutureVote, vote.Slot, currentSlot)
	}

	return nil
}

// ProcessAttestation handles a new attestation vote from network gossip.
func (s *Store) ProcessAttestation(signedVote *types.SignedVote) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateAttestationLocked(signedVote); err != nil {
		return err
	}
	s.processAttestationLocked(signedVote, false)
	return nil
}

func (s *Store) processAttestationLocked(signedVote *types.SignedVote, isFromBlock bool) {
	vote := signedVote.Data
	idx := vote.ValidatorID

	if isFromBlock {
		known := s.LatestKnownVotes[idx]
		if known.Root.IsZero() || known.Slot < vote.Slot {
			s.LatestKnownVotes[idx] = vote.Target
		}
		newVote := s.LatestNewVotes[idx]
		if !newVote.Root.IsZero() && newVote.Slot <= vote.Target.Slot {
			s.LatestNewVotes[idx] = types.Checkpoint{}
		}
	} else {
		newVote := s.LatestNewVotes[idx]
		if newVote.Root.IsZero() || newVote.Slot < vote.Target.Slot {
			s.LatestNewVotes[idx] = vote.Target
		}
	}
}

func (s *Store) acceptNewVotesLocked() {
	for i, vote := range s.LatestNewVotes {
		if !vote.Root.IsZero() {
			s.LatestKnownVotes[i] = vote
			s.LatestNewVotes[i] = types.Checkpoint{}
		}
	}
	s.updateHeadLocked()
}

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
