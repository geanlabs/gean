package forkchoice

import (
	"fmt"
	"sync"

	"github.com/devylongs/gean/chain"
	"github.com/devylongs/gean/types"
)

// Store tracks all information required for the LMD GHOST fork choice algorithm.
type Store struct {
	mu sync.RWMutex

	Time            uint64
	Config          types.Config
	Head            types.Root
	SafeTarget      types.Root
	LatestJustified types.Checkpoint
	LatestFinalized types.Checkpoint

	Blocks           map[types.Root]*types.Block
	States           map[types.Root]*types.State
	LatestKnownVotes map[types.ValidatorIndex]types.Checkpoint
	LatestNewVotes   map[types.ValidatorIndex]types.Checkpoint
}

// RLock acquires a read lock on the store.
func (s *Store) RLock() {
	s.mu.RLock()
}

// RUnlock releases a read lock on the store.
func (s *Store) RUnlock() {
	s.mu.RUnlock()
}

// NewStore initializes a fork choice store from an anchor state and block.
func NewStore(state *types.State, anchorBlock *types.Block) (*Store, error) {
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash state: %w", err)
	}

	if anchorBlock.StateRoot != stateRoot {
		return nil, fmt.Errorf("anchor block state root mismatch")
	}

	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash anchor block: %w", err)
	}

	// Initialize justified/finalized checkpoints with anchor block root
	// This ensures votes can reference the genesis as their source
	latestJustified := types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot}
	latestFinalized := types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot}

	return &Store{
		Time:             uint64(anchorBlock.Slot) * types.IntervalsPerSlot,
		Config:           state.Config,
		Head:             anchorRoot,
		SafeTarget:       anchorRoot,
		LatestJustified:  latestJustified,
		LatestFinalized:  latestFinalized,
		Blocks:           map[types.Root]*types.Block{anchorRoot: anchorBlock},
		States:           map[types.Root]*types.State{anchorRoot: state},
		LatestKnownVotes: make(map[types.ValidatorIndex]types.Checkpoint),
		LatestNewVotes:   make(map[types.ValidatorIndex]types.Checkpoint),
	}, nil
}

// ProcessBlock adds a new block and updates fork choice state.
func (s *Store) ProcessBlock(block *types.Block) error {
	blockHash, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash block: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip if already known
	if _, exists := s.Blocks[blockHash]; exists {
		return nil
	}

	// Get parent state
	parentState, exists := s.States[block.ParentRoot]
	if !exists {
		return fmt.Errorf("parent state not found")
	}

	// Apply state transition
	newState, err := chain.ProcessSlots(parentState, block.Slot)
	if err != nil {
		return fmt.Errorf("process slots: %w", err)
	}
	newState, err = chain.ProcessBlock(newState, block)
	if err != nil {
		return fmt.Errorf("process block: %w", err)
	}

	// Store block and state
	s.Blocks[blockHash] = block
	s.States[blockHash] = newState

	// Process attestations
	for _, signedVote := range block.Body.Attestations {
		s.processAttestationLocked(&signedVote, true)
	}

	// Update head
	s.updateHeadLocked()
	return nil
}

// ValidateAttestation validates an attestation according to Devnet 0 spec.
func (s *Store) ValidateAttestation(signedVote *types.SignedVote) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validateAttestationLocked(signedVote)
}

func (s *Store) validateAttestationLocked(signedVote *types.SignedVote) error {
	vote := signedVote.Data

	// Validate vote targets exist in store
	if _, exists := s.Blocks[vote.Source.Root]; !exists {
		return fmt.Errorf("source root not found in store")
	}
	if _, exists := s.Blocks[vote.Target.Root]; !exists {
		return fmt.Errorf("target root not found in store")
	}

	sourceBlock := s.Blocks[vote.Source.Root]
	targetBlock := s.Blocks[vote.Target.Root]

	// Validate slot relationships
	if sourceBlock.Slot > targetBlock.Slot {
		return fmt.Errorf("source block slot %d > target block slot %d", sourceBlock.Slot, targetBlock.Slot)
	}
	if vote.Source.Slot > vote.Target.Slot {
		return fmt.Errorf("source slot %d > target slot %d", vote.Source.Slot, vote.Target.Slot)
	}

	// Validate checkpoint slots match block slots
	if sourceBlock.Slot != vote.Source.Slot {
		return fmt.Errorf("source block slot %d != checkpoint slot %d", sourceBlock.Slot, vote.Source.Slot)
	}
	if targetBlock.Slot != vote.Target.Slot {
		return fmt.Errorf("target block slot %d != checkpoint slot %d", targetBlock.Slot, vote.Target.Slot)
	}

	// Validate attestation is not too far in future
	currentSlot := types.Slot(s.Time / types.IntervalsPerSlot)
	if vote.Slot > currentSlot+1 {
		return fmt.Errorf("vote slot %d too far in future (current: %d)", vote.Slot, currentSlot)
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

// processAttestationLocked handles a new attestation vote. Caller must hold lock.
func (s *Store) processAttestationLocked(signedVote *types.SignedVote, isFromBlock bool) {
	vote := signedVote.Data
	validatorID := types.ValidatorIndex(vote.ValidatorID)

	if isFromBlock {
		// On-chain attestation
		if known, exists := s.LatestKnownVotes[validatorID]; !exists || known.Slot < vote.Slot {
			s.LatestKnownVotes[validatorID] = vote.Target
		}
		if newVote, exists := s.LatestNewVotes[validatorID]; exists && newVote.Slot <= vote.Target.Slot {
			delete(s.LatestNewVotes, validatorID)
		}
	} else {
		// Network gossip attestation
		if newVote, exists := s.LatestNewVotes[validatorID]; !exists || newVote.Slot < vote.Target.Slot {
			s.LatestNewVotes[validatorID] = vote.Target
		}
	}
}

// UpdateHead updates the store's head based on latest justified checkpoint and votes.
func (s *Store) UpdateHead() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateHeadLocked()
}

// updateHeadLocked updates head. Caller must hold lock.
func (s *Store) updateHeadLocked() {
	if latest := GetLatestJustified(s.States); latest != nil {
		// Only update LatestJustified if we have the block in our store
		// This prevents referencing blocks we haven't received yet
		if _, exists := s.Blocks[latest.Root]; exists {
			s.LatestJustified = *latest
		}
	}

	s.Head = GetHead(s.Blocks, s.LatestJustified.Root, s.LatestKnownVotes, 0)

	if state, exists := s.States[s.Head]; exists {
		// Only update LatestFinalized if we have the block in our store
		if _, exists := s.Blocks[state.LatestFinalized.Root]; exists {
			s.LatestFinalized = state.LatestFinalized
		}
	}
}

// AcceptNewVotes moves pending votes to known votes and updates head.
func (s *Store) AcceptNewVotes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptNewVotesLocked()
}

// acceptNewVotesLocked moves pending votes. Caller must hold lock.
func (s *Store) acceptNewVotesLocked() {
	for validatorID, vote := range s.LatestNewVotes {
		s.LatestKnownVotes[validatorID] = vote
	}
	s.LatestNewVotes = make(map[types.ValidatorIndex]types.Checkpoint)
	s.updateHeadLocked()
}

// UpdateSafeTarget calculates the safe target with 2/3 majority threshold.
func (s *Store) UpdateSafeTarget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateSafeTargetLocked()
}

// updateSafeTargetLocked calculates safe target. Caller must hold lock.
func (s *Store) updateSafeTargetLocked() {
	minScore := int((s.Config.NumValidators*2 + 2) / 3) // ceiling division
	s.SafeTarget = GetHead(s.Blocks, s.LatestJustified.Root, s.LatestNewVotes, minScore)
}

// TickInterval advances store time by one interval.
func (s *Store) TickInterval(hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickIntervalLocked(hasProposal)
}

// tickIntervalLocked advances time. Caller must hold lock.
func (s *Store) tickIntervalLocked(hasProposal bool) {
	s.Time++
	currentInterval := s.Time % types.IntervalsPerSlot

	switch currentInterval {
	case 0:
		if hasProposal {
			s.acceptNewVotesLocked()
		}
	case 1:
		// Validator voting interval - no action
	case 2:
		s.updateSafeTargetLocked()
	default:
		s.acceptNewVotesLocked()
	}
}

// AdvanceTime ticks the store forward to the given time.
func (s *Store) AdvanceTime(time uint64, hasProposal bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Don't advance time if we're before genesis
	if time < s.Config.GenesisTime {
		return
	}

	tickIntervalTime := (time - s.Config.GenesisTime) / types.SecondsPerInterval

	for s.Time < tickIntervalTime {
		shouldSignal := hasProposal && (s.Time+1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
}

// GetProposalHead returns the head for block proposal at the given slot.
func (s *Store) GetProposalHead(slot types.Slot) types.Root {
	s.mu.Lock()
	defer s.mu.Unlock()

	slotTime := s.Config.GenesisTime + uint64(slot)*types.SecondsPerSlot
	tickIntervalTime := (slotTime - s.Config.GenesisTime) / types.SecondsPerInterval
	for s.Time < tickIntervalTime {
		shouldSignal := (s.Time + 1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
	s.acceptNewVotesLocked()
	return s.Head
}

// GetVoteTarget calculates the target checkpoint for validator votes.
func (s *Store) GetVoteTarget() types.Checkpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// CurrentSlot returns the current slot based on store time.
func (s *Store) CurrentSlot() types.Slot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return types.Slot(s.Time / types.IntervalsPerSlot)
}

// ProduceBlock creates a new block for the given slot and validator.
// It iteratively collects valid attestations and computes the state root.
func (s *Store) ProduceBlock(slot types.Slot, validatorIndex types.ValidatorIndex) (*types.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate proposer authorization
	expectedProposer := uint64(slot) % s.Config.NumValidators
	if uint64(validatorIndex) != expectedProposer {
		return nil, fmt.Errorf("validator %d is not the proposer for slot %d (expected %d)",
			validatorIndex, slot, expectedProposer)
	}

	// Get parent block and state (inline GetProposalHead logic to avoid deadlock)
	slotTime := s.Config.GenesisTime + uint64(slot)*types.SecondsPerSlot
	tickIntervalTime := (slotTime - s.Config.GenesisTime) / types.SecondsPerInterval
	for s.Time < tickIntervalTime {
		shouldSignal := (s.Time + 1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
	s.acceptNewVotesLocked()
	headRoot := s.Head

	headState, exists := s.States[headRoot]
	if !exists {
		return nil, fmt.Errorf("head state not found")
	}

	// Iteratively collect valid attestations
	var attestations []types.SignedVote

	for {
		// Create candidate block
		candidateBlock := &types.Block{
			Slot:          slot,
			ProposerIndex: uint64(validatorIndex),
			ParentRoot:    headRoot,
			StateRoot:     types.Root{}, // Temporary
			Body:          types.BlockBody{Attestations: attestations},
		}

		// Apply state transition to get post-block state
		advancedState, err := chain.ProcessSlots(headState, slot)
		if err != nil {
			return nil, fmt.Errorf("process slots: %w", err)
		}
		postState, err := chain.ProcessBlock(advancedState, candidateBlock)
		if err != nil {
			return nil, fmt.Errorf("process block: %w", err)
		}

		// Find new valid attestations
		var newAttestations []types.SignedVote
		for validatorID, checkpoint := range s.LatestKnownVotes {
			// Skip if target block unknown
			if _, exists := s.Blocks[checkpoint.Root]; !exists {
				continue
			}

			// Create attestation with post-state's latest justified as source
			vote := types.Vote{
				ValidatorID: uint64(validatorID),
				Slot:        checkpoint.Slot,
				Head:        checkpoint,
				Target:      checkpoint,
				Source:      postState.LatestJustified,
			}
			signedVote := types.SignedVote{Data: vote, Signature: types.Root{}}

			// Check if already in attestation set
			found := false
			for _, existing := range attestations {
				if existing.Data.ValidatorID == signedVote.Data.ValidatorID &&
					existing.Data.Slot == signedVote.Data.Slot {
					found = true
					break
				}
			}
			if !found {
				newAttestations = append(newAttestations, signedVote)
			}
		}

		// Fixed point reached
		if len(newAttestations) == 0 {
			break
		}

		attestations = append(attestations, newAttestations...)
	}

	// Create final block
	finalState, err := chain.ProcessSlots(headState, slot)
	if err != nil {
		return nil, fmt.Errorf("process slots for final block: %w", err)
	}

	finalBlock := &types.Block{
		Slot:          slot,
		ProposerIndex: uint64(validatorIndex),
		ParentRoot:    headRoot,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: attestations},
	}

	// Apply state transition and compute state root
	finalPostState, err := chain.ProcessBlock(finalState, finalBlock)
	if err != nil {
		return nil, fmt.Errorf("process final block: %w", err)
	}

	stateRoot, err := finalPostState.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash final state: %w", err)
	}
	finalBlock.StateRoot = stateRoot

	// Store block and state
	blockHash, err := finalBlock.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash final block: %w", err)
	}
	s.Blocks[blockHash] = finalBlock
	s.States[blockHash] = finalPostState

	return finalBlock, nil
}

// ProduceAttestationVote creates an attestation vote for the given slot and validator.
func (s *Store) ProduceAttestationVote(slot types.Slot, validatorIndex types.ValidatorIndex) *types.Vote {
	s.mu.Lock()

	// Get the head block for this slot (inline GetProposalHead logic)
	slotTime := s.Config.GenesisTime + uint64(slot)*types.SecondsPerSlot
	tickIntervalTime := (slotTime - s.Config.GenesisTime) / types.SecondsPerInterval
	for s.Time < tickIntervalTime {
		shouldSignal := (s.Time + 1) == tickIntervalTime
		s.tickIntervalLocked(shouldSignal)
	}
	s.acceptNewVotesLocked()
	headRoot := s.Head
	headBlock := s.Blocks[headRoot]

	headCheckpoint := types.Checkpoint{
		Root: headRoot,
		Slot: headBlock.Slot,
	}

	// Get vote target data while holding lock
	targetRoot := s.Head
	for i := 0; i < 3; i++ {
		if s.Blocks[targetRoot].Slot > s.Blocks[s.SafeTarget].Slot {
			targetRoot = s.Blocks[targetRoot].ParentRoot
		}
	}
	for !s.Blocks[targetRoot].Slot.IsJustifiableAfter(s.LatestFinalized.Slot) {
		targetRoot = s.Blocks[targetRoot].ParentRoot
	}
	block := s.Blocks[targetRoot]
	blockRoot, _ := block.HashTreeRoot()
	targetCheckpoint := types.Checkpoint{Root: blockRoot, Slot: block.Slot}

	latestJustified := s.LatestJustified
	s.mu.Unlock()

	// Create the vote
	return &types.Vote{
		ValidatorID: uint64(validatorIndex),
		Slot:        slot,
		Head:        headCheckpoint,
		Target:      targetCheckpoint,
		Source:      latestJustified,
	}
}
