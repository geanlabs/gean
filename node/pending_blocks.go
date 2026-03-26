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
	mu              sync.Mutex
	blocks          map[[32]byte]*types.SignedBlockWithAttestation
	byParent        map[[32]byte][][32]byte // parent root -> child block roots
	missingAncestor map[[32]byte][32]byte   // block root -> deepest unresolved ancestor root
	order           [][32]byte              // insertion order for eviction
}

// NewPendingBlockCache creates an empty pending block cache.
func NewPendingBlockCache() *PendingBlockCache {
	return &PendingBlockCache{
		blocks:          make(map[[32]byte]*types.SignedBlockWithAttestation),
		byParent:        make(map[[32]byte][][32]byte),
		missingAncestor: make(map[[32]byte][32]byte),
	}
}

// Add stores a block that is awaiting its parent.
// If the cache is full, the oldest entry is evicted.
func (c *PendingBlockCache) Add(sb *types.SignedBlockWithAttestation) {
	if sb == nil || sb.Message == nil || sb.Message.Block == nil {
		return
	}
	c.AddWithMissingAncestor(sb, sb.Message.Block.ParentRoot)
}

// AddWithMissingAncestor stores a block alongside the deepest unresolved
// ancestor root currently preventing its import.
func (c *PendingBlockCache) AddWithMissingAncestor(sb *types.SignedBlockWithAttestation, missingAncestor [32]byte) {
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
		c.missingAncestor[blockRoot] = missingAncestor
		return
	}

	// Evict oldest if at capacity.
	for len(c.order) >= maxPendingBlocks {
		oldest := c.order[0]
		c.order = c.order[1:]
		if oldBlock, ok := c.blocks[oldest]; ok {
			delete(c.blocks, oldest)
			delete(c.missingAncestor, oldest)
			oldParent := oldBlock.Message.Block.ParentRoot
			c.removeFromParentIndex(oldParent, oldest)
		}
	}

	c.blocks[blockRoot] = sb
	c.missingAncestor[blockRoot] = missingAncestor
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
	delete(c.missingAncestor, blockRoot)
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

// MissingAncestor returns the deepest unresolved ancestor root recorded for a
// cached block.
func (c *PendingBlockCache) MissingAncestor(blockRoot [32]byte) ([32]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	root, ok := c.missingAncestor[blockRoot]
	return root, ok
}

// MissingParents returns unique unresolved ancestor roots for pending blocks.
// The caller must still check whether each root is now available in fork
// choice, since this cache only tracks pending-block lineage.
func (c *PendingBlockCache) MissingParents() [][32]byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[[32]byte]struct{})
	var missing [][32]byte
	for _, missingRoot := range c.missingAncestor {
		if _, dup := seen[missingRoot]; !dup {
			seen[missingRoot] = struct{}{}
			missing = append(missing, missingRoot)
		}
	}
	return missing
}

// PruneFinalized removes all pending blocks at or below the given slot.
// Returns the number of blocks pruned.
func (c *PendingBlockCache) PruneFinalized(finalizedSlot uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	for root, sb := range c.blocks {
		if sb.Message.Block.Slot <= finalizedSlot {
			delete(c.blocks, root)
			delete(c.missingAncestor, root)
			c.removeFromParentIndex(sb.Message.Block.ParentRoot, root)
			pruned++
		}
	}

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
