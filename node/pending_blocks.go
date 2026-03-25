package node

import (
	"slices"
	"sync"

	"github.com/geanlabs/gean/types"
)

const maxPendingBlocks = 1024

// PendingBlockCache stores blocks awaiting parent availability.
// Per leanSpec sync requirements, blocks with missing parents should be cached,
// not discarded, allowing the node to process them once the parent arrives.
type PendingBlockCache struct {
	mu       sync.Mutex
	blocks   map[[32]byte]*types.SignedBlockWithAttestation
	byParent map[[32]byte][][32]byte // parent root -> child block roots
	order    [][32]byte              // insertion order for eviction
}

// NewPendingBlockCache creates an empty pending block cache.
func NewPendingBlockCache() *PendingBlockCache {
	return &PendingBlockCache{
		blocks:   make(map[[32]byte]*types.SignedBlockWithAttestation),
		byParent: make(map[[32]byte][][32]byte),
	}
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
		}
	}

	c.blocks[blockRoot] = sb
	c.byParent[parentRoot] = append(c.byParent[parentRoot], blockRoot)
	c.order = append(c.order, blockRoot)
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
