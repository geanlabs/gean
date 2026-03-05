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

	latestKnownAttestations map[uint64]*types.SignedAttestation
	latestNewAttestations   map[uint64]*types.SignedAttestation
	gossipSignatures        map[signatureKey]storedSignature
	aggregatedPayloads      map[signatureKey][]storedAggregatedPayload

	NowFn            func() uint64
	OnBlockProcessed func()
}

// FCSnapshot is a serialisable snapshot of the fork-choice metadata.
type FCSnapshot struct {
	Head          [32]byte
	SafeTarget    [32]byte
	JustifiedRoot [32]byte
	JustifiedSlot uint64
	FinalizedRoot [32]byte
	FinalizedSlot uint64
	GenesisTime   uint64
	Time          uint64
	NumValidators uint64
	Attestations  map[uint64]*types.SignedAttestation
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

// RestoreStore reconstructs a fork-choice store from a persisted snapshot.
// The underlying storage.Store must already contain the blocks and states.
func RestoreStore(snap *FCSnapshot, store storage.Store) *Store {
	known := make(map[uint64]*types.SignedAttestation, len(snap.Attestations))
	for k, v := range snap.Attestations {
		known[k] = v
	}
	return &Store{
		time:                    snap.Time,
		genesisTime:             snap.GenesisTime,
		numValidators:           snap.NumValidators,
		head:                    snap.Head,
		safeTarget:              snap.SafeTarget,
		latestJustified:         &types.Checkpoint{Root: snap.JustifiedRoot, Slot: snap.JustifiedSlot},
		latestFinalized:         &types.Checkpoint{Root: snap.FinalizedRoot, Slot: snap.FinalizedSlot},
		storage:                 store,
		latestKnownAttestations: known,
		latestNewAttestations:   make(map[uint64]*types.SignedAttestation),
		gossipSignatures:        make(map[signatureKey]storedSignature),
		aggregatedPayloads:      make(map[signatureKey][]storedAggregatedPayload),
	}
}

// GetSnapshotLocked returns a snapshot of the fork-choice metadata.
// The caller MUST hold c.mu.
func (c *Store) GetSnapshotLocked() FCSnapshot {
	atts := make(map[uint64]*types.SignedAttestation, len(c.latestKnownAttestations))
	for k, v := range c.latestKnownAttestations {
		atts[k] = v
	}
	return FCSnapshot{
		Head:          c.head,
		SafeTarget:    c.safeTarget,
		JustifiedRoot: c.latestJustified.Root,
		JustifiedSlot: c.latestJustified.Slot,
		FinalizedRoot: c.latestFinalized.Root,
		FinalizedSlot: c.latestFinalized.Slot,
		GenesisTime:   c.genesisTime,
		Time:          c.time,
		NumValidators: c.numValidators,
		Attestations:  atts,
	}
}

// GetSnapshot returns a snapshot of the fork-choice metadata (acquires lock).
func (c *Store) GetSnapshot() FCSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.GetSnapshotLocked()
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
		time:                    anchorBlock.Slot * types.SecondsPerSlot,
		genesisTime:             state.Config.GenesisTime,
		numValidators:           uint64(len(state.Validators)),
		head:                    anchorRoot,
		safeTarget:              anchorRoot,
		latestJustified:         &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		latestFinalized:         &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		storage:                 store,
		latestKnownAttestations: make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:   make(map[uint64]*types.SignedAttestation),
		gossipSignatures:        make(map[signatureKey]storedSignature),
		aggregatedPayloads:      make(map[signatureKey][]storedAggregatedPayload),
	}
}
