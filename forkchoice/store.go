package forkchoice

import (
	"fmt"
	"sync"

	"github.com/devylongs/gean/clock"
	"github.com/devylongs/gean/types"
)

// ProcessSlotsFunc applies per-slot processing up to a target slot.
type ProcessSlotsFunc func(state *types.State, targetSlot types.Slot) (*types.State, error)

// ProcessBlockFunc applies block processing to a state.
type ProcessBlockFunc func(state *types.State, block *types.Block) (*types.State, error)

// Store maintains fork choice state including blocks, states, and votes.
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
}

// NewStore creates a new fork choice store with the given genesis state, anchor block,
// and injected state transition functions.
func NewStore(
	state *types.State,
	anchorBlock *types.Block,
	processSlots ProcessSlotsFunc,
	processBlock ProcessBlockFunc,
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

	// Per leanSpec get_forkchoice_store: use state's checkpoints, not anchor block
	latestJustified := state.LatestJustified
	latestFinalized := state.LatestFinalized

	return &Store{
		Clock:            clock.New(state.Config.GenesisTime, anchorBlock.Slot),
		Config:           state.Config,
		Head:             anchorRoot,
		SafeTarget:       anchorRoot,
		LatestJustified:  latestJustified,
		LatestFinalized:  latestFinalized,
		Blocks:           map[types.Root]*types.Block{anchorRoot: anchorBlock},
		States:           map[types.Root]*types.State{anchorRoot: state},
		LatestKnownVotes: make([]types.Checkpoint, state.Config.NumValidators),
		LatestNewVotes:   make([]types.Checkpoint, state.Config.NumValidators),
		processSlots:     processSlots,
		processBlock:     processBlock,
	}, nil
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

	// Process attestations
	for _, signedVote := range block.Body.Attestations {
		s.processAttestationLocked(&signedVote, true)
	}

	// Update head
	s.updateHeadLocked()
	return nil
}

func (s *Store) updateHeadLocked() {
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
}

func (s *Store) updateSafeTargetLocked() {
	minScore := int((s.Config.NumValidators*2 + 2) / 3) // ceiling division
	s.SafeTarget = GetHead(s.Blocks, s.LatestJustified.Root, s.LatestNewVotes, minScore)
}
