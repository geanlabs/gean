package forkchoice

import (
	"fmt"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// GetProposalHead returns the head for block proposal at the given slot.
func (c *Store) GetProposalHead(slot uint64) [32]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	slotTime := c.GenesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	return c.Head
}

// GetVoteTarget calculates the target checkpoint for validator votes.
func (c *Store) GetVoteTarget() *types.Checkpoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getVoteTargetLocked()
}

func (c *Store) getVoteTargetLocked() *types.Checkpoint {
	blocks := c.Storage.GetAllBlocks()
	targetRoot := c.Head

	// Walk back up to JustificationLookback steps if safe target is newer.
	for i := 0; i < types.JustificationLookback; i++ {
		tBlock, ok := blocks[targetRoot]
		sBlock, ok2 := blocks[c.SafeTarget]
		if ok && ok2 && tBlock.Slot > sBlock.Slot {
			targetRoot = tBlock.ParentRoot
		}
	}

	// Ensure target is in justifiable slot range.
	for {
		tBlock, ok := blocks[targetRoot]
		if !ok {
			break
		}
		if types.IsJustifiableAfter(tBlock.Slot, c.LatestFinalized.Slot) {
			break
		}
		targetRoot = tBlock.ParentRoot
	}

	tBlock, ok := blocks[targetRoot]
	if !ok {
		panic("vote target block not found")
	}
	blockHash, _ := tBlock.HashTreeRoot()
	return &types.Checkpoint{Root: blockHash, Slot: tBlock.Slot}
}

// ProduceBlock creates a new block for the given slot and validator.
func (c *Store) ProduceBlock(slot, validatorIndex uint64) (*types.Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !statetransition.IsProposer(validatorIndex, slot, c.NumValidators) {
		return nil, fmt.Errorf("validator %d is not proposer for slot %d", validatorIndex, slot)
	}

	headRoot := c.Head
	// Advance and accept before proposing.
	slotTime := c.GenesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot = c.Head

	headState, ok := c.Storage.GetState(headRoot)
	if !ok {
		return nil, fmt.Errorf("head state not found")
	}

	var attestations []*types.Attestation

	// Fixed-point attestation collection.
	for {
		candidateBlock := &types.Block{
			Slot:          slot,
			ProposerIndex: validatorIndex,
			ParentRoot:    headRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: attestations},
		}

		advancedState, err := statetransition.ProcessSlots(headState, slot)
		if err != nil {
			return nil, err
		}
		postState, err := statetransition.ProcessBlock(advancedState, candidateBlock)
		if err != nil {
			return nil, err
		}

		var newAttestations []*types.Attestation
		for _, att := range c.LatestKnownAttestations {
			if _, ok := c.Storage.GetBlock(att.Data.Head.Root); !ok {
				continue
			}
			// Skip attestations whose source doesn't match post-state justified.
			if att.Data.Source.Root != postState.LatestJustified.Root ||
				att.Data.Source.Slot != postState.LatestJustified.Slot {
				continue
			}
			if !containsAttestation(attestations, att) {
				newAttestations = append(newAttestations, att)
			}
		}

		if len(newAttestations) == 0 {
			break
		}
		attestations = append(attestations, newAttestations...)
	}

	// Build final block with computed state root.
	finalAdvanced, err := statetransition.ProcessSlots(headState, slot)
	if err != nil {
		return nil, err
	}
	finalBlock := &types.Block{
		Slot:          slot,
		ProposerIndex: validatorIndex,
		ParentRoot:    headRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: attestations},
	}
	finalState, err := statetransition.ProcessBlock(finalAdvanced, finalBlock)
	if err != nil {
		return nil, err
	}
	stateRoot, _ := finalState.HashTreeRoot()
	finalBlock.StateRoot = stateRoot

	blockHash, _ := finalBlock.HashTreeRoot()
	c.Storage.PutBlock(blockHash, finalBlock)
	c.Storage.PutState(blockHash, finalState)

	return finalBlock, nil
}

// ProduceAttestation produces an attestation for the given slot and validator.
func (c *Store) ProduceAttestation(slot, validatorIndex uint64) *types.Attestation {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Advance and accept before voting (matches leanSpec produce_attestation_vote).
	slotTime := c.GenesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot := c.Head

	blocks := c.Storage.GetAllBlocks()
	headBlock, ok := blocks[headRoot]
	if !ok {
		panic("head block not found")
	}

	headCheckpoint := &types.Checkpoint{Root: headRoot, Slot: headBlock.Slot}
	targetCheckpoint := c.getVoteTargetLocked()

	return &types.Attestation{
		ValidatorID: validatorIndex,
		Data: &types.AttestationData{
			Slot:   slot,
			Head:   headCheckpoint,
			Target: targetCheckpoint,
			Source: c.LatestJustified,
		},
	}
}
