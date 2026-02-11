package forkchoice

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/devylongs/gean/clock"
	"github.com/devylongs/gean/types"
)

// StoreOption configures optional Store parameters.
type StoreOption func(*Store)

// WithLogger sets the logger for consensus event logging.
func WithLogger(logger *slog.Logger) StoreOption {
	return func(s *Store) { s.logger = logger }
}

// ProcessSlotsFunc applies per-slot processing up to a target slot.
// Injected from the consensus package to avoid circular imports.
type ProcessSlotsFunc func(state *types.State, targetSlot types.Slot) (*types.State, error)

// ProcessBlockFunc applies block processing to a state.
// Injected from the consensus package to avoid circular imports.
type ProcessBlockFunc func(state *types.State, block *types.Block) (*types.State, error)

// Store maintains fork choice state: blocks, states, votes, and checkpoints.
// Votes are split into "known" (accepted) and "new" (pending) for safe head calculation.
type Store struct {
	mu sync.RWMutex

	Clock           *clock.SlotClock
	Config          types.Config
	Head            types.Root
	SafeTarget      types.Root
	LatestJustified types.Checkpoint
	LatestFinalized types.Checkpoint

	Blocks           map[types.Root]*types.Block
	States           map[types.Root]*types.State
	LatestKnownVotes []types.Checkpoint // indexed by ValidatorIndex
	LatestNewVotes   []types.Checkpoint // indexed by ValidatorIndex

	// Injected state transition functions (from consensus package).
	processSlots ProcessSlotsFunc
	processBlock ProcessBlockFunc

	logger *slog.Logger
}

// NewStore creates a new fork choice store from an anchor state and block.
func NewStore(
	state *types.State,
	anchorBlock *types.Block,
	processSlots ProcessSlotsFunc,
	processBlock ProcessBlockFunc,
	opts ...StoreOption,
) (*Store, error) {
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

	// Use state's checkpoints, not anchor block's
	latestJustified := state.LatestJustified
	latestFinalized := state.LatestFinalized

	store := &Store{
		Clock:            clock.New(state.Config.GenesisTime, anchorBlock.Slot),
		Config:           state.Config,
		Head:             anchorRoot,
		SafeTarget:       anchorRoot,
		LatestJustified:  latestJustified,
		LatestFinalized:  latestFinalized,
		Blocks:           map[types.Root]*types.Block{anchorRoot: anchorBlock},
		States:           map[types.Root]*types.State{anchorRoot: state},
		LatestKnownVotes: make([]types.Checkpoint, len(state.Validators)),
		LatestNewVotes:   make([]types.Checkpoint, len(state.Validators)),
		processSlots:     processSlots,
		processBlock:     processBlock,
		logger:           slog.Default(),
	}

	for _, opt := range opts {
		opt(store)
	}

	return store, nil
}

// HasBlock checks if a block exists in the store.
func (s *Store) HasBlock(root types.Root) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.Blocks[root]
	return exists
}

// GetBlock retrieves a block from the store.
func (s *Store) GetBlock(root types.Root) (*types.Block, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	block, exists := s.Blocks[root]
	return block, exists
}

// GetHead returns the current head block root.
func (s *Store) GetHead() types.Root {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Head
}

// GetLatestFinalized returns the latest finalized checkpoint.
func (s *Store) GetLatestFinalized() types.Checkpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LatestFinalized
}

// GetLatestJustified returns the latest justified checkpoint.
func (s *Store) GetLatestJustified() types.Checkpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LatestJustified
}

// ProcessBlock validates and adds a new block to the store, then updates fork choice.
// Note: the spec's store.process_block omits process_slots before process_block,
// which appears to be a spec bug — we correctly advance slots first.
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
		return fmt.Errorf("%w: parent root %x", ErrParentNotFound, block.ParentRoot[:8])
	}

	// Apply state transition (via injected functions)
	newState, err := s.processSlots(parentState, block.Slot)
	if err != nil {
		return fmt.Errorf("process slots: %w", err)
	}
	newState, err = s.processBlock(newState, block)
	if err != nil {
		return fmt.Errorf("process block: %w", err)
	}

	// Validate state root matches the block's declared state root
	computedStateRoot, err := newState.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash post-state: %w", err)
	}
	if block.StateRoot != computedStateRoot {
		return fmt.Errorf("invalid block state root: got %x, computed %x",
			block.StateRoot[:8], computedStateRoot[:8])
	}

	// Store block and state
	s.Blocks[blockHash] = block
	s.States[blockHash] = newState

	// Process attestations from the block body (wrap unsigned attestations for the store)
	for _, att := range block.Body.Attestations {
		signedAtt := &types.SignedAttestation{Message: att}
		s.processAttestationLocked(signedAtt, true)
	}

	// Update head
	s.updateHeadLocked()
	return nil
}

// updateHeadLocked recalculates the canonical head and updates justified/finalized checkpoints.
func (s *Store) updateHeadLocked() {
	prevHead := s.Head
	prevJustified := s.LatestJustified
	prevFinalized := s.LatestFinalized

	if latest := GetLatestJustified(s.States); latest != nil {
		// Only update LatestJustified if we have the block in our store
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

	// Log consensus events
	if s.Head != prevHead {
		s.logger.Info("head updated",
			"root", s.Head.Short(),
			"slot", s.Blocks[s.Head].Slot,
		)
	}
	if s.LatestJustified.Slot > prevJustified.Slot {
		s.logger.Info("justified checkpoint advanced",
			"slot", s.LatestJustified.Slot,
			"root", s.LatestJustified.Root.Short(),
		)
	}
	if s.LatestFinalized.Slot > prevFinalized.Slot {
		s.logger.Info("finalized checkpoint advanced",
			"slot", s.LatestFinalized.Slot,
			"root", s.LatestFinalized.Root.Short(),
		)
	}
}

// updateSafeTargetLocked recalculates the safe target using 2/3 supermajority on new votes.
func (s *Store) updateSafeTargetLocked() {
	numValidators := uint64(len(s.LatestKnownVotes))
	minScore := int((numValidators*2 + 2) / 3) // ceiling division
	s.SafeTarget = GetHead(s.Blocks, s.LatestJustified.Root, s.LatestNewVotes, minScore)
}
