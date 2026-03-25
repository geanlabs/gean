package node

import (
	"log/slog"
	"slices"
	"sync"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

const maxPendingBlocks = 1024

// PendingBlockCache stores blocks awaiting parent availability.
// Blocks are persisted to the database so they survive restarts.
type PendingBlockCache struct {
	mu       sync.Mutex
	blocks   map[[32]byte]*types.SignedBlockWithAttestation
	byParent map[[32]byte][][32]byte // parent root -> child block roots
	order    [][32]byte              // insertion order for eviction
	store    storage.Store
}

// NewPendingBlockCache creates an empty pending block cache backed by the given store.
func NewPendingBlockCache(store storage.Store) *PendingBlockCache {
	return &PendingBlockCache{
		blocks:   make(map[[32]byte]*types.SignedBlockWithAttestation),
		byParent: make(map[[32]byte][][32]byte),
		store:    store,
	}
}

// LoadFromDB populates the in-memory cache from persisted pending blocks.
// Called once during startup to restore state after a restart.
func (c *PendingBlockCache) LoadFromDB(log *slog.Logger) {
	all := c.store.GetAllPendingBlocks()
	if len(all) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for root, sb := range all {
		if sb == nil || sb.Message == nil || sb.Message.Block == nil {
			continue
		}
		if _, ok := c.blocks[root]; ok {
			continue
		}
		c.blocks[root] = sb
		parentRoot := sb.Message.Block.ParentRoot
		c.byParent[parentRoot] = append(c.byParent[parentRoot], root)
		c.order = append(c.order, root)
	}

	log.Info("restored pending blocks from database", "count", len(c.blocks))
}

// Add stores a block that is awaiting its parent.
// If the cache is full, the oldest entry is evicted.
func (c *PendingBlockCache) Add(sb *types.SignedBlockWithAttestation) {
	if sb == nil || sb.Message == nil || sb.Message.Block == nil {
		return
	}

	block := sb.Message.Block
	blockRoot, _ := block.HashTreeRoot()
	parentRoot := block.ParentRoot

	c.mu.Lock()
	defer c.mu.Unlock()

	// Already cached.
	if _, ok := c.blocks[blockRoot]; ok {
		return
	}

	// Evict oldest if at capacity.
	for len(c.order) >= maxPendingBlocks {
		oldest := c.order[0]
		c.order = c.order[1:]
		if oldBlock, ok := c.blocks[oldest]; ok {
			delete(c.blocks, oldest)
			oldParent := oldBlock.Message.Block.ParentRoot
			c.removeFromParentIndex(oldParent, oldest)
			c.store.DeletePendingBlock(oldest)
		}
	}

	c.blocks[blockRoot] = sb
	c.byParent[parentRoot] = append(c.byParent[parentRoot], blockRoot)
	c.order = append(c.order, blockRoot)
	c.store.PutPendingBlock(blockRoot, sb)
}

// GetChildrenOf returns all pending blocks that have the given root as their parent.
func (c *PendingBlockCache) GetChildrenOf(parentRoot [32]byte) []*types.SignedBlockWithAttestation {
	c.mu.Lock()
	defer c.mu.Unlock()

	childRoots := c.byParent[parentRoot]
	if len(childRoots) == 0 {
		return nil
	}

	children := make([]*types.SignedBlockWithAttestation, 0, len(childRoots))
	for _, root := range childRoots {
		if sb, ok := c.blocks[root]; ok {
			children = append(children, sb)
		}
	}
	return children
}

// Remove deletes a block from the cache (called after successful processing).
func (c *PendingBlockCache) Remove(blockRoot [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sb, ok := c.blocks[blockRoot]
	if !ok {
		return
	}

	delete(c.blocks, blockRoot)
	parentRoot := sb.Message.Block.ParentRoot
	c.removeFromParentIndex(parentRoot, blockRoot)

	// Remove from order slice.
	for i, r := range c.order {
		if r == blockRoot {
			c.order = slices.Delete(c.order, i, i+1)
			break
		}
	}

	c.store.DeletePendingBlock(blockRoot)
}

// PruneFinalized removes all pending blocks at or before the given slot.
func (c *PendingBlockCache) PruneFinalized(finalizedSlot uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	for root, sb := range c.blocks {
		if sb.Message.Block.Slot <= finalizedSlot {
			delete(c.blocks, root)
			c.removeFromParentIndex(sb.Message.Block.ParentRoot, root)
			c.store.DeletePendingBlock(root)
			pruned++
		}
	}

	// Rebuild order slice if anything was pruned.
	if pruned > 0 {
		newOrder := c.order[:0]
		for _, r := range c.order {
			if _, ok := c.blocks[r]; ok {
				newOrder = append(newOrder, r)
			}
		}
		c.order = newOrder
	}
	return pruned
}

// removeFromParentIndex removes a block root from the byParent index.
// Must be called with lock held.
func (c *PendingBlockCache) removeFromParentIndex(parentRoot, blockRoot [32]byte) {
	children := c.byParent[parentRoot]
	for i, r := range children {
		if r == blockRoot {
			c.byParent[parentRoot] = slices.Delete(children, i, i+1)
			break
		}
	}
	if len(c.byParent[parentRoot]) == 0 {
		delete(c.byParent, parentRoot)
	}
}

func (c *PendingBlockCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.blocks)
}

// Has returns true if the block with the given root is in the cache.
func (c *PendingBlockCache) Has(blockRoot [32]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.blocks[blockRoot]
	return ok
}
