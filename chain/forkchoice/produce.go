package forkchoice

import (
	"fmt"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// Signer abstracts the signing capability (XMSS or mock).
type Signer interface {
	Sign(epoch uint32, message [32]byte) ([]byte, error)
}

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
func (c *Store) GetVoteTarget() (*types.Checkpoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getVoteTargetLocked()
}

func (c *Store) getVoteTargetLocked() (*types.Checkpoint, error) {
	targetRoot := c.Head

	// Walk back up to JustificationLookback steps if safe target is newer.
	safeBlock, safeOK := c.Storage.GetBlock(c.SafeTarget)
	for i := 0; i < types.JustificationLookback; i++ {
		tBlock, ok := c.Storage.GetBlock(targetRoot)
		if ok && safeOK && tBlock.Slot > safeBlock.Slot {
			targetRoot = tBlock.ParentRoot
		}
	}

	// Ensure target is in justifiable slot range.
	for {
		tBlock, ok := c.Storage.GetBlock(targetRoot)
		if !ok {
			break
		}
		if types.IsJustifiableAfter(tBlock.Slot, c.LatestFinalized.Slot) {
			break
		}
		targetRoot = tBlock.ParentRoot
	}

	tBlock, ok := c.Storage.GetBlock(targetRoot)
	if !ok {
		return nil, fmt.Errorf("vote target block not found")
	}
	blockHash, _ := tBlock.HashTreeRoot()
	return &types.Checkpoint{Root: blockHash, Slot: tBlock.Slot}, nil
}

// ProduceBlock creates a new signed block envelope for the given slot and validator.
// The returned envelope includes:
//   - the block with body attestations
//   - the proposer's own attestation (head = produced block)
//   - the signature list (body attestation sigs + proposer sig last)
//
// The signer is used to produce the proposer's XMSS signature over the
// BlockWithAttestation hash-tree-root.
func (c *Store) ProduceBlock(slot, validatorIndex uint64, signer Signer) (*types.SignedBlockWithAttestation, error) {
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
	var collectedSigned []*types.SignedAttestation

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
		var newSigned []*types.SignedAttestation
		for _, sa := range c.LatestKnownAttestations {
			data := sa.Message
			if _, ok := c.Storage.GetBlock(data.Head.Root); !ok {
				continue
			}
			// Skip attestations whose source doesn't match post-state justified.
			if data.Source.Root != postState.LatestJustified.Root ||
				data.Source.Slot != postState.LatestJustified.Slot {
				continue
			}
			att := &types.Attestation{
				ValidatorID: sa.ValidatorID,
				Data:        data,
			}
			if !containsAttestation(attestations, att) {
				newAttestations = append(newAttestations, att)
				newSigned = append(newSigned, sa)
			}
		}

		if len(newAttestations) == 0 {
			break
		}
		attestations = append(attestations, newAttestations...)
		collectedSigned = append(collectedSigned, newSigned...)
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

	// Build proposer attestation: the proposer attests to its own block.
	proposerAtt := &types.Attestation{
		ValidatorID: validatorIndex,
		Data: &types.AttestationData{
			Slot:   slot,
			Head:   &types.Checkpoint{Root: blockHash, Slot: slot},
			Source: c.LatestJustified,
		},
	}
	voteTarget, err := c.getVoteTargetLocked()
	if err != nil {
		return nil, fmt.Errorf("vote target: %w", err)
	}
	proposerAtt.Data.Target = voteTarget

	// Build signature list: body attestation sigs in order, proposer sig last.
	sigs := make([][3112]byte, len(collectedSigned)+1)
	for i, sa := range collectedSigned {
		sigs[i] = sa.Signature
	}

	envelope := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block:               finalBlock,
			ProposerAttestation: proposerAtt,
		},
		Signature: sigs,
	}

	// Sign the BlockWithAttestation root with the proposer's key.
	msgRoot, err := envelope.Message.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash block-with-attestation: %w", err)
	}
	epoch := uint32(slot / types.SlotsPerEpoch)
	sig, err := signer.Sign(epoch, msgRoot)
	if err != nil {
		return nil, fmt.Errorf("sign block: %w", err)
	}
	copy(envelope.Signature[len(collectedSigned)][:], sig)

	c.Storage.PutBlock(blockHash, finalBlock)
	c.Storage.PutSignedBlock(blockHash, envelope)
	c.Storage.PutState(blockHash, finalState)

	return envelope, nil
}

// ProduceAttestation produces a signed attestation for the given slot and validator.
// The signer produces the XMSS signature over HashTreeRoot(AttestationData).
func (c *Store) ProduceAttestation(slot, validatorIndex uint64, signer Signer) (*types.SignedAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Advance and accept before voting (matches leanSpec produce_attestation_vote).
	slotTime := c.GenesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot := c.Head

	headBlock, ok := c.Storage.GetBlock(headRoot)
	if !ok {
		return nil, fmt.Errorf("head block not found")
	}

	headCheckpoint := &types.Checkpoint{Root: headRoot, Slot: headBlock.Slot}
	targetCheckpoint, err := c.getVoteTargetLocked()
	if err != nil {
		return nil, fmt.Errorf("vote target: %w", err)
	}

	data := &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: targetCheckpoint,
		Source: c.LatestJustified,
	}

	// Sign the attestation data root.
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash attestation data: %w", err)
	}
	epoch := uint32(data.Target.Slot / types.SlotsPerEpoch)
	sig, err := signer.Sign(epoch, dataRoot)
	if err != nil {
		return nil, fmt.Errorf("sign attestation: %w", err)
	}

	var sigBytes [3112]byte
	copy(sigBytes[:], sig)

	return &types.SignedAttestation{
		ValidatorID: validatorIndex,
		Message:     data,
		Signature:   sigBytes,
	}, nil
}
