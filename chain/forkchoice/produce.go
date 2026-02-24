package forkchoice

import (
	"fmt"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// Signer abstracts the signing capability (XMSS or mock).
type Signer interface {
	Sign(signingSlot uint32, message [32]byte) ([]byte, error)
}

// GetProposalHead returns the head for block proposal at the given slot.
func (c *Store) GetProposalHead(slot uint64) [32]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	slotTime := c.genesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	return c.head
}

// GetVoteTarget calculates the target checkpoint for validator votes.
func (c *Store) GetVoteTarget() (*types.Checkpoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getVoteTargetLocked()
}

func (c *Store) getVoteTargetLocked() (*types.Checkpoint, error) {
	targetRoot := c.head

	// Walk back up to JustificationLookback steps if safe target is newer.
	safeBlock, safeOK := c.storage.GetBlock(c.safeTarget)
	for i := 0; i < types.JustificationLookback; i++ {
		tBlock, ok := c.storage.GetBlock(targetRoot)
		if ok && safeOK && tBlock.Slot > safeBlock.Slot {
			targetRoot = tBlock.ParentRoot
		}
	}

	// Ensure target is in justifiable slot range.
	for {
		tBlock, ok := c.storage.GetBlock(targetRoot)
		if !ok {
			break
		}
		if types.IsJustifiableAfter(tBlock.Slot, c.latestFinalized.Slot) {
			break
		}
		targetRoot = tBlock.ParentRoot
	}

	tBlock, ok := c.storage.GetBlock(targetRoot)
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
// proposer attestation hash-tree-root.
func (c *Store) ProduceBlock(slot, validatorIndex uint64, signer Signer) (*types.SignedBlockWithAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !statetransition.IsProposer(validatorIndex, slot, c.numValidators) {
		return nil, fmt.Errorf("validator %d is not proposer for slot %d", validatorIndex, slot)
	}

	headRoot := c.head
	// Advance and accept before proposing.
	slotTime := c.genesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot = c.head

	headState, ok := c.storage.GetState(headRoot)
	if !ok {
		return nil, fmt.Errorf("head state not found")
	}

	advancedState, err := statetransition.ProcessSlots(headState, slot)
	if err != nil {
		return nil, err
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

		postState, err := statetransition.ProcessBlock(advancedState, candidateBlock)
		if err != nil {
			return nil, err
		}

		var newAttestations []*types.Attestation
		var newSigned []*types.SignedAttestation
		for _, sa := range c.latestKnownAttestations {
			data := sa.Message.Data
			if _, ok := c.storage.GetBlock(data.Head.Root); !ok {
				continue
			}
			// Skip attestations whose source doesn't match post-state justified.
			if data.Source.Root != postState.LatestJustified.Root ||
				data.Source.Slot != postState.LatestJustified.Slot {
				continue
			}
			if !containsAttestation(attestations, sa.Message) {
				newAttestations = append(newAttestations, sa.Message)
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
	finalBlock := &types.Block{
		Slot:          slot,
		ProposerIndex: validatorIndex,
		ParentRoot:    headRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: attestations},
	}
	finalState, err := statetransition.ProcessBlock(advancedState, finalBlock)
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
			Source: c.latestJustified,
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

	// Sign proposer attestation message (validator_id + data).
	msgRoot, err := proposerAtt.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash proposer attestation: %w", err)
	}
	signingSlot := uint32(proposerAtt.Data.Slot)
	sig, err := signer.Sign(signingSlot, msgRoot)
	if err != nil {
		return nil, fmt.Errorf("sign proposer attestation: %w", err)
	}
	copy(envelope.Signature[len(collectedSigned)][:], sig)

	c.storage.PutBlock(blockHash, finalBlock)
	c.storage.PutSignedBlock(blockHash, envelope)
	c.storage.PutState(blockHash, finalState)

	return envelope, nil
}

// ProduceAttestation produces a signed attestation for the given slot and validator.
// The signer produces the XMSS signature over HashTreeRoot(Attestation).
func (c *Store) ProduceAttestation(slot, validatorIndex uint64, signer Signer) (*types.SignedAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Advance and accept before voting (matches leanSpec produce_attestation_vote).
	slotTime := c.genesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot := c.head

	headBlock, ok := c.storage.GetBlock(headRoot)
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
		Source: c.latestJustified,
	}

	att := &types.Attestation{
		ValidatorID: validatorIndex,
		Data:        data,
	}

	// Sign the attestation message root (validator_id + data).
	messageRoot, err := att.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash attestation: %w", err)
	}
	signingSlot := uint32(data.Slot)
	sig, err := signer.Sign(signingSlot, messageRoot)
	if err != nil {
		return nil, fmt.Errorf("sign attestation: %w", err)
	}

	var sigBytes [3112]byte
	copy(sigBytes[:], sig)

	return &types.SignedAttestation{
		Message:   att,
		Signature: sigBytes,
	}, nil
}
