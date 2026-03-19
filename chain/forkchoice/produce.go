package forkchoice

import (
	"fmt"
	"sort"

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

	// Ensure target is at or after the source (latest_justified) to maintain invariant:
	// source.slot <= target.slot. This prevents creating invalid attestations where
	// source slot exceeds target slot. If the calculated target is older than
	// latest_justified, use latest_justified instead.
	if tBlock.Slot < c.latestJustified.Slot {
		return c.latestJustified, nil
	}

	return &types.Checkpoint{Root: targetRoot, Slot: tBlock.Slot}, nil
}

// ProduceBlock creates a new devnet-2 signed block envelope for the given slot.
func (c *Store) ProduceBlock(slot, validatorIndex uint64, signer Signer) (*types.SignedBlockWithAttestation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !statetransition.IsProposer(validatorIndex, slot, c.numValidators) {
		return nil, fmt.Errorf("validator %d is not proposer for slot %d", validatorIndex, slot)
	}

	slotTime := c.genesisTime + slot*types.SecondsPerSlot
	c.advanceTimeLocked(slotTime, true)
	c.acceptNewAttestationsLocked()
	headRoot := c.head

	headState, ok := c.storage.GetState(headRoot)
	if !ok {
		return nil, fmt.Errorf("head state not found")
	}

	advancedState, err := statetransition.ProcessSlots(headState, slot)
	if err != nil {
		return nil, err
	}

	selectedByValidator := make(map[uint64]*types.SignedAttestation)
	selected := make([]*types.SignedAttestation, 0, len(c.latestKnownAttestations))

	// Fixed-point collection: include votes whose source matches post-state justified.
	for {
		aggregatedAttestations, _, err := c.buildAggregatedAttestationsFromSigned(headState, selected)
		if err != nil {
			return nil, err
		}

		candidate := &types.Block{
			Slot:          slot,
			ProposerIndex: validatorIndex,
			ParentRoot:    headRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: aggregatedAttestations},
		}

		postState, err := statetransition.ProcessBlock(advancedState, candidate)
		if err != nil {
			return nil, err
		}

		added := false
		for _, sa := range c.latestKnownAttestations {
			if sa == nil || sa.Message == nil || sa.Message.Source == nil || sa.Message.Head == nil {
				continue
			}
			if _, ok := c.storage.GetBlock(sa.Message.Head.Root); !ok {
				continue
			}
			if sa.Message.Source.Root != postState.LatestJustified.Root ||
				sa.Message.Source.Slot != postState.LatestJustified.Slot {
				continue
			}

			existing, ok := selectedByValidator[sa.ValidatorID]
			if ok && existing != nil && existing.Message != nil && existing.Message.Slot >= sa.Message.Slot {
				continue
			}
			selectedByValidator[sa.ValidatorID] = sa
			added = true
		}

		if !added {
			break
		}
		selected = orderedSignedAttestations(selectedByValidator)
	}

	finalAttestations, attestationProofs, err := c.buildAggregatedAttestationsFromSigned(headState, selected)
	if err != nil {
		return nil, err
	}

	finalBlock := &types.Block{
		Slot:          slot,
		ProposerIndex: validatorIndex,
		ParentRoot:    headRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: finalAttestations},
	}
	finalState, err := statetransition.ProcessBlock(advancedState, finalBlock)
	if err != nil {
		return nil, err
	}
	stateRoot, _ := finalState.HashTreeRoot()
	finalBlock.StateRoot = stateRoot

	blockHash, _ := finalBlock.HashTreeRoot()
	voteTarget, err := c.getVoteTargetLocked()
	if err != nil {
		return nil, fmt.Errorf("vote target: %w", err)
	}

	proposerAtt := &types.Attestation{
		ValidatorID: validatorIndex,
		Data: &types.AttestationData{
			Slot:   slot,
			Head:   &types.Checkpoint{Root: blockHash, Slot: slot},
			Target: voteTarget,
			Source: &types.Checkpoint{Root: c.latestJustified.Root, Slot: c.latestJustified.Slot},
		},
	}

	envelope := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block:               finalBlock,
			ProposerAttestation: proposerAtt,
		},
		Signature: types.BlockSignatures{
			AttestationSignatures: attestationProofs,
		},
	}

	messageRoot, err := proposerAtt.Data.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash proposer attestation data: %w", err)
	}
	sig, err := signer.Sign(uint32(proposerAtt.Data.Slot), messageRoot)
	if err != nil {
		return nil, fmt.Errorf("sign proposer attestation: %w", err)
	}
	copy(envelope.Signature.ProposerSignature[:], sig)

	return envelope, nil
}

// ProduceAttestation produces a signed attestation for the given slot and validator.
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

	// Cannot produce valid attestation if target is strictly before the source.
	// target == source is valid (e.g. genesis bootstrap where both are slot 0).
	if targetCheckpoint.Slot < c.latestJustified.Slot {
		return nil, fmt.Errorf("cannot produce valid attestation: target slot %d < source slot %d",
			targetCheckpoint.Slot, c.latestJustified.Slot)
	}

	data := &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: targetCheckpoint,
		Source: &types.Checkpoint{Root: c.latestJustified.Root, Slot: c.latestJustified.Slot},
	}

	messageRoot, err := data.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash attestation data: %w", err)
	}
	sig, err := signer.Sign(uint32(data.Slot), messageRoot)
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

func orderedSignedAttestations(indexed map[uint64]*types.SignedAttestation) []*types.SignedAttestation {
	if len(indexed) == 0 {
		return nil
	}
	validatorIDs := make([]uint64, 0, len(indexed))
	for validatorID := range indexed {
		validatorIDs = append(validatorIDs, validatorID)
	}
	sort.Slice(validatorIDs, func(i, j int) bool { return validatorIDs[i] < validatorIDs[j] })

	out := make([]*types.SignedAttestation, 0, len(indexed))
	for _, validatorID := range validatorIDs {
		out = append(out, indexed[validatorID])
	}
	return out
}
