package forkchoice

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

// ProcessBlock processes a new block and updates chain state.
func (c *Store) ProcessBlock(block *types.Block) error {
	start := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	blockHash, _ := block.HashTreeRoot()

	if _, ok := c.Storage.GetBlock(blockHash); ok {
		return nil // already known
	}

	parentState, ok := c.Storage.GetState(block.ParentRoot)
	if !ok {
		return fmt.Errorf("parent state not found for %x", block.ParentRoot)
	}

	signedBlock := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
	state, err := statetransition.StateTransition(parentState, signedBlock)
	if err != nil {
		return fmt.Errorf("state_transition: %w", err)
	}

	c.Storage.PutBlock(blockHash, block)
	c.Storage.PutState(blockHash, state)

	// Process block attestations as on-chain votes.
	for _, sv := range block.Body.Attestations {
		c.processAttestationLocked(sv, true)
	}

	c.updateHeadLocked()
	metrics.ForkChoiceBlockProcessingTime.Observe(time.Since(start).Seconds())
	metrics.StateTransitionTime.Observe(time.Since(start).Seconds())
	return nil
}
