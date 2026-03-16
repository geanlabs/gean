package forkchoice

import (
	"fmt"
	"sync"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

var log = logging.NewComponentLogger(logging.CompForkChoice)

// Store tracks chain state and validator votes for the LMD GHOST algorithm.
type Store struct {
	mu sync.Mutex

	time          uint64
	genesisTime   uint64
	numValidators uint64
	head          [32]byte
	safeTarget    [32]byte

	latestJustified *types.Checkpoint
	latestFinalized *types.Checkpoint
	storage         storage.Store
	isAggregator    bool

	latestKnownAttestations       map[uint64]*types.SignedAttestation
	latestNewAttestations         map[uint64]*types.SignedAttestation
	latestKnownAggregatedPayloads map[[32]byte]aggregatedPayload
	latestNewAggregatedPayloads   map[[32]byte]aggregatedPayload
	gossipSignatures              map[signatureKey]storedSignature
	aggregatedPayloads            map[signatureKey][]storedAggregatedPayload

	NowFn func() uint64
}

// ChainStatus is a snapshot of the fork choice head and checkpoint state.
type ChainStatus struct {
	Head          [32]byte
	HeadSlot      uint64
	JustifiedRoot [32]byte
	JustifiedSlot uint64
	FinalizedRoot [32]byte
	FinalizedSlot uint64
}

// GetStatus returns a consistent snapshot of the chain head and checkpoints.
func (c *Store) GetStatus() ChainStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	headSlot := uint64(0)
	if hb, ok := c.storage.GetBlock(c.head); ok {
		headSlot = hb.Slot
	}
	return ChainStatus{
		Head:          c.head,
		HeadSlot:      headSlot,
		JustifiedRoot: c.latestJustified.Root,
		JustifiedSlot: c.latestJustified.Slot,
		FinalizedRoot: c.latestFinalized.Root,
		FinalizedSlot: c.latestFinalized.Slot,
	}
}

// NumValidators returns the number of validators in the store.
func (c *Store) NumValidators() uint64 {
	return c.numValidators
}

// GetBlock retrieves a block by its root hash.
func (c *Store) GetBlock(root [32]byte) (*types.Block, bool) {
	return c.storage.GetBlock(root)
}

// GetSignedBlock retrieves a signed block envelope by its root hash.
func (c *Store) GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool) {
	return c.storage.GetSignedBlock(root)
}

// HasState returns true if the state for the given block root exists.
// This is used by sync to verify chain connectivity: ProcessBlock requires
// the parent state, not just the parent block, to succeed.
func (c *Store) HasState(root [32]byte) bool {
	_, ok := c.storage.GetState(root)
	return ok
}

// GetKnownAttestation returns the latest known attestation for a validator.
func (c *Store) GetKnownAttestation(validator uint64) (*types.SignedAttestation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sa, ok := c.latestKnownAttestations[validator]
	return sa, ok
}

// GetNewAttestation returns the latest new (pending) attestation for a validator.
func (c *Store) GetNewAttestation(validator uint64) (*types.SignedAttestation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sa, ok := c.latestNewAttestations[validator]
	return sa, ok
}

// SetIsAggregator configures whether this store's node acts as an aggregator.
func (c *Store) SetIsAggregator(isAggregator bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isAggregator = isAggregator
}

// RestoreFromDB reconstructs a fork-choice store from persisted blocks and states.
// It finds the highest-slot block as the head and derives checkpoints from its state.
// Returns nil if the database is empty.
func RestoreFromDB(store storage.Store) *Store {
	allBlocks := store.GetAllBlocks()
	if len(allBlocks) == 0 {
		return nil
	}

	// Find the block with the highest slot (chain head).
	var headRoot [32]byte
	var headBlock *types.Block
	for root, blk := range allBlocks {
		if headBlock == nil || blk.Slot > headBlock.Slot {
			headRoot = root
			headBlock = blk
		}
	}

	headState, ok := store.GetState(headRoot)
	if !ok {
		return nil
	}

	return &Store{
		time:                          headBlock.Slot * types.IntervalsPerSlot,
		genesisTime:                   headState.Config.GenesisTime,
		numValidators:                 uint64(len(headState.Validators)),
		head:                          headRoot,
		safeTarget:                    headState.LatestFinalized.Root,
		latestJustified:               headState.LatestJustified,
		latestFinalized:               headState.LatestFinalized,
		storage:                       store,
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
	}
}

// NewStore initializes a store from an anchor state and block.
func NewStore(state *types.State, anchorBlock *types.Block, store storage.Store) *Store {
	stateRoot, _ := state.HashTreeRoot()
	if anchorBlock.StateRoot != stateRoot {
		panic(fmt.Sprintf("anchor block state root mismatch: block=%x state=%x", anchorBlock.StateRoot, stateRoot))
	}

	anchorRoot, _ := anchorBlock.HashTreeRoot()

	store.PutBlock(anchorRoot, anchorBlock)
	store.PutSignedBlock(anchorRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: anchorBlock},
	})
	store.PutState(anchorRoot, state)

	return &Store{
		time:                          anchorBlock.Slot * types.IntervalsPerSlot,
		genesisTime:                   state.Config.GenesisTime,
		numValidators:                 uint64(len(state.Validators)),
		head:                          anchorRoot,
		safeTarget:                    anchorRoot,
		latestJustified:               &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		latestFinalized:               &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		storage:                       store,
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
	}
}
