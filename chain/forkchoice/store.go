package forkchoice

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

var log = logging.NewComponentLogger(logging.CompForkChoice)

const (
	blocksToKeep = 21600
	statesToKeep = 3000
)

type blockSummary struct {
	Slot          uint64
	ParentRoot    [32]byte
	ProposerIndex uint64
}

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
	blockSummaries  map[[32]byte]blockSummary
	checkpointRoots map[[32]byte]blockSummary
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
	if head, ok := c.lookupBlockSummary(c.head); ok {
		headSlot = head.Slot
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

	blockSummaries := make(map[[32]byte]blockSummary, len(allBlocks))
	for root, block := range allBlocks {
		blockSummaries[root] = summarizeBlock(block)
	}
	metrics.ForkchoiceBlockSummaryRoots.Set(float64(len(blockSummaries)))

	return &Store{
		time:                          headBlock.Slot * types.IntervalsPerSlot,
		genesisTime:                   headState.Config.GenesisTime,
		numValidators:                 uint64(len(headState.Validators)),
		head:                          headRoot,
		safeTarget:                    headState.LatestFinalized.Root,
		latestJustified:               headState.LatestJustified,
		latestFinalized:               headState.LatestFinalized,
		storage:                       store,
		blockSummaries:                blockSummaries,
		checkpointRoots:               buildCheckpointRootIndex(headState, headRoot),
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
	metrics.ForkchoiceBlockSummaryRoots.Set(1)

	return &Store{
		time:                          anchorBlock.Slot * types.IntervalsPerSlot,
		genesisTime:                   state.Config.GenesisTime,
		numValidators:                 uint64(len(state.Validators)),
		head:                          anchorRoot,
		safeTarget:                    anchorRoot,
		latestJustified:               &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		latestFinalized:               &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot},
		storage:                       store,
		blockSummaries:                map[[32]byte]blockSummary{anchorRoot: summarizeBlock(anchorBlock)},
		checkpointRoots:               nil,
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
	}
}

// NewStoreFromCheckpointState initializes a store from a checkpoint state.
// The checkpoint state is expected to have a latest block header whose
// hash-tree-root matches anchorRoot after the header state root has been set.
func NewStoreFromCheckpointState(state *types.State, anchorRoot [32]byte, store storage.Store) *Store {
	anchorHeader := state.LatestBlockHeader
	anchorBlock := &types.Block{
		Slot:          anchorHeader.Slot,
		ProposerIndex: anchorHeader.ProposerIndex,
		ParentRoot:    anchorHeader.ParentRoot,
		StateRoot:     anchorHeader.StateRoot,
		Body:          emptyCheckpointBody(),
	}

	store.PutBlock(anchorRoot, anchorBlock)
	store.PutSignedBlock(anchorRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: anchorBlock},
	})
	store.PutState(anchorRoot, state)
	metrics.ForkchoiceBlockSummaryRoots.Set(1)

	return &Store{
		time:                          state.Slot * types.IntervalsPerSlot,
		genesisTime:                   state.Config.GenesisTime,
		numValidators:                 uint64(len(state.Validators)),
		head:                          anchorRoot,
		safeTarget:                    state.LatestFinalized.Root,
		latestJustified:               &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot},
		latestFinalized:               &types.Checkpoint{Root: state.LatestFinalized.Root, Slot: state.LatestFinalized.Slot},
		storage:                       store,
		blockSummaries:                map[[32]byte]blockSummary{anchorRoot: summarizeBlock(anchorBlock)},
		checkpointRoots:               buildCheckpointRootIndex(state, anchorRoot),
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
	}
}

func emptyCheckpointBody() *types.BlockBody {
	return &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
}

func summarizeBlock(block *types.Block) blockSummary {
	return blockSummary{
		Slot:          block.Slot,
		ParentRoot:    block.ParentRoot,
		ProposerIndex: block.ProposerIndex,
	}
}

func buildCheckpointRootIndex(state *types.State, anchorRoot [32]byte) map[[32]byte]blockSummary {
	if state == nil || state.LatestBlockHeader == nil {
		return nil
	}

	refs := make(map[[32]byte]blockSummary, len(state.HistoricalBlockHashes)+1)
	lastNonZeroRoot := types.ZeroHash
	for slot, root := range state.HistoricalBlockHashes {
		if root == types.ZeroHash {
			continue
		}
		refs[root] = blockSummary{
			Slot:       uint64(slot),
			ParentRoot: lastNonZeroRoot,
		}
		lastNonZeroRoot = root
	}

	refs[anchorRoot] = blockSummary{
		Slot:          state.LatestBlockHeader.Slot,
		ParentRoot:    state.LatestBlockHeader.ParentRoot,
		ProposerIndex: state.LatestBlockHeader.ProposerIndex,
	}
	return refs
}

func (c *Store) lookupBlockSummary(root [32]byte) (blockSummary, bool) {
	if c.blockSummaries != nil {
		if summary, ok := c.blockSummaries[root]; ok {
			return summary, true
		}
	}
	if c.checkpointRoots == nil {
		return blockSummary{}, false
	}
	summary, ok := c.checkpointRoots[root]
	return summary, ok
}

func (c *Store) allKnownBlockSummaries() map[[32]byte]blockSummary {
	summaries := make(map[[32]byte]blockSummary, len(c.blockSummaries)+len(c.checkpointRoots))
	for root, summary := range c.blockSummaries {
		summaries[root] = summary
	}
	for root, summary := range c.checkpointRoots {
		if _, ok := summaries[root]; !ok {
			summaries[root] = summary
		}
	}
	return summaries
}

func (c *Store) PruneOldData() (prunedBlocks int, prunedStates int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pruneOldDataLocked(blocksToKeep, statesToKeep)
}

func (c *Store) pruneOldDataLocked(blockLimit int, stateLimit int) (prunedBlocks int, prunedStates int) {
	if len(c.blockSummaries) == 0 {
		return 0, 0
	}

	type rootWithSummary struct {
		root    [32]byte
		summary blockSummary
	}

	ordered := make([]rootWithSummary, 0, len(c.blockSummaries))
	for root, summary := range c.blockSummaries {
		ordered = append(ordered, rootWithSummary{root: root, summary: summary})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].summary.Slot != ordered[j].summary.Slot {
			return ordered[i].summary.Slot > ordered[j].summary.Slot
		}
		return bytes.Compare(ordered[i].root[:], ordered[j].root[:]) < 0
	})

	protected := map[[32]byte]struct{}{
		c.head:                 {},
		c.safeTarget:           {},
		c.latestJustified.Root: {},
		c.latestFinalized.Root: {},
	}

	keepBlocks := make(map[[32]byte]struct{}, min(blockLimit, len(ordered))+len(protected))
	keepStates := make(map[[32]byte]struct{}, min(stateLimit, len(ordered))+len(protected))
	for root := range protected {
		keepBlocks[root] = struct{}{}
		keepStates[root] = struct{}{}
	}
	for i, item := range ordered {
		if i < blockLimit {
			keepBlocks[item.root] = struct{}{}
		}
		if i < stateLimit {
			keepStates[item.root] = struct{}{}
		}
	}

	blockRootsToDelete := make([][32]byte, 0, len(ordered))
	stateRootsToDelete := make([][32]byte, 0, len(ordered))
	for _, item := range ordered {
		if _, keep := keepBlocks[item.root]; !keep {
			blockRootsToDelete = append(blockRootsToDelete, item.root)
		}
		if _, keep := keepStates[item.root]; !keep {
			stateRootsToDelete = append(stateRootsToDelete, item.root)
		}
	}

	if len(blockRootsToDelete) > 0 {
		c.storage.DeleteBlocks(blockRootsToDelete)
		for _, root := range blockRootsToDelete {
			delete(c.blockSummaries, root)
		}
	}
	if len(stateRootsToDelete) > 0 {
		c.storage.DeleteStates(stateRootsToDelete)
	}
	metrics.ForkchoiceBlockSummaryRoots.Set(float64(len(c.blockSummaries)))

	return len(blockRootsToDelete), len(stateRootsToDelete)
}
