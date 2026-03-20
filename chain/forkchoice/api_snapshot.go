package forkchoice

import (
	"sort"

	"github.com/geanlabs/gean/types"
)

// ForkChoiceNode is a lightweight view of a block for API responses.
type ForkChoiceNode struct {
	Root          [32]byte
	Slot          uint64
	ParentRoot    [32]byte
	ProposerIndex uint64
	Weight        int
}

// ForkChoiceSnapshot is a read-only snapshot of fork choice state.
type ForkChoiceSnapshot struct {
	Nodes          []ForkChoiceNode
	Head           [32]byte
	SafeTarget     [32]byte
	Justified      types.Checkpoint
	Finalized      types.Checkpoint
	ValidatorCount uint64
}

// ForkChoiceSnapshot returns a consistent fork choice snapshot for API responses.
func (c *Store) ForkChoiceSnapshot() ForkChoiceSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	blocks := c.allKnownBlockSummaries()
	weights := computeBlockWeights(blocks, c.latestKnownAttestations)

	finalized := types.Checkpoint{}
	if c.latestFinalized != nil {
		finalized = *c.latestFinalized
	}
	justified := types.Checkpoint{}
	if c.latestJustified != nil {
		justified = *c.latestJustified
	}

	nodes := make([]ForkChoiceNode, 0, len(blocks))
	for root, block := range blocks {
		if block.Slot < finalized.Slot {
			continue
		}
		nodes = append(nodes, ForkChoiceNode{
			Root:          root,
			Slot:          block.Slot,
			ParentRoot:    block.ParentRoot,
			ProposerIndex: block.ProposerIndex,
			Weight:        weights[root],
		})
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Slot != nodes[j].Slot {
			return nodes[i].Slot < nodes[j].Slot
		}
		return hashGreater(nodes[i].Root, nodes[j].Root)
	})

	validatorCount := uint64(0)
	if headState, ok := c.storage.GetState(c.head); ok && headState != nil {
		validatorCount = uint64(len(headState.Validators))
	}

	return ForkChoiceSnapshot{
		Nodes:          nodes,
		Head:           c.head,
		SafeTarget:     c.safeTarget,
		Justified:      justified,
		Finalized:      finalized,
		ValidatorCount: validatorCount,
	}
}

// FinalizedStateSSZ returns SSZ bytes for the latest finalized state.
// ok is false when the finalized state is not available.
func (c *Store) FinalizedStateSSZ() ([]byte, bool, error) {
	c.mu.Lock()
	if c.latestFinalized == nil {
		c.mu.Unlock()
		return nil, false, nil
	}
	root := c.latestFinalized.Root
	c.mu.Unlock()

	state, ok := c.storage.GetState(root)
	if !ok || state == nil {
		return nil, false, nil
	}

	sszBytes, err := state.MarshalSSZ()
	if err != nil {
		return nil, true, err
	}
	return sszBytes, true, nil
}

func computeBlockWeights(blocks map[[32]byte]blockSummary, latestAttestations map[uint64]*types.SignedAttestation) map[[32]byte]int {
	weights := make(map[[32]byte]int, len(blocks))
	for _, sa := range latestAttestations {
		if sa == nil || sa.Message == nil || sa.Message.Head == nil {
			continue
		}
		headRoot := sa.Message.Head.Root
		if _, ok := blocks[headRoot]; !ok {
			continue
		}
		blockHash := headRoot
		for {
			b, ok := blocks[blockHash]
			if !ok {
				break
			}
			weights[blockHash]++
			if b.Slot == 0 {
				break
			}
			blockHash = b.ParentRoot
		}
	}
	return weights
}
