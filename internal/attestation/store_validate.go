package attestation

import (
	"fmt"
	"math"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func errUnknownSourceBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownSourceBlock, Message: fmt.Sprintf("unknown source block: %x", root[:4])}
}

func errUnknownTargetBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownTargetBlock, Message: fmt.Sprintf("unknown target block: %x", root[:4])}
}

func errUnknownHeadBlock(root [32]byte) error {
	return &store.StoreError{Kind: store.ErrUnknownHeadBlock, Message: fmt.Sprintf("unknown head block: %x", root[:4])}
}

func errSourceExceedsTarget() error {
	return &store.StoreError{Kind: store.ErrSourceExceedsTarget, Message: "source checkpoint slot exceeds target"}
}

func errHeadOlderThanTarget(headSlot, targetSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadOlderThanTarget, Message: fmt.Sprintf("head slot %d older than target slot %d", headSlot, targetSlot)}
}

func errSourceSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrSourceSlotMismatch, Message: fmt.Sprintf("source checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errTargetSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrTargetSlotMismatch, Message: fmt.Sprintf("target checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errHeadSlotMismatch(cpSlot, blockSlot uint64) error {
	return &store.StoreError{Kind: store.ErrHeadSlotMismatch, Message: fmt.Sprintf("head checkpoint slot %d != block slot %d", cpSlot, blockSlot)}
}

func errAttestationTooFarInFuture(attSlot, storeTime uint64) error {
	return &store.StoreError{Kind: store.ErrAttestationTooFarInFuture, Message: fmt.Sprintf("attestation slot %d too far in future (store time %d intervals)", attSlot, storeTime)}
}

// ValidateAttestationData checks 9 validation branches for incoming attestations.
func ValidateAttestationData(s *store.ConsensusStore, data *types.AttestationData) error {
	// 1-3. Availability: source, target, head blocks must exist.
	sourceHeader := s.GetBlockHeader(data.Source.Root)
	if sourceHeader == nil {
		return errUnknownSourceBlock(data.Source.Root)
	}
	targetHeader := s.GetBlockHeader(data.Target.Root)
	if targetHeader == nil {
		return errUnknownTargetBlock(data.Target.Root)
	}
	headHeader := s.GetBlockHeader(data.Head.Root)
	if headHeader == nil {
		return errUnknownHeadBlock(data.Head.Root)
	}

	// 4. Topology: source.slot <= target.slot.
	if data.Source.Slot > data.Target.Slot {
		return errSourceExceedsTarget()
	}

	// 5. Topology: head.slot >= target.slot.
	if data.Head.Slot < data.Target.Slot {
		return errHeadOlderThanTarget(data.Head.Slot, data.Target.Slot)
	}

	// 6-8. Consistency: checkpoint slots match actual block slots.
	if sourceHeader.Slot != data.Source.Slot {
		return errSourceSlotMismatch(data.Source.Slot, sourceHeader.Slot)
	}
	if targetHeader.Slot != data.Target.Slot {
		return errTargetSlotMismatch(data.Target.Slot, targetHeader.Slot)
	}
	if headHeader.Slot != data.Head.Slot {
		return errHeadSlotMismatch(data.Head.Slot, headHeader.Slot)
	}

	// 9. Time: attestation's start interval must not exceed store time by
	// more than GossipDisparityIntervals (clock-skew tolerance, ~800ms).
	// The bound is in intervals, not slots — a whole-slot margin would let an
	// adversary pre-publish next-slot aggregates ahead of any honest
	// validator. The first conjunct guards against uint64 overflow for
	// malicious slot values near MaxUint64.
	if data.Slot > math.MaxUint64/types.IntervalsPerSlot ||
		data.Slot*types.IntervalsPerSlot > s.Time()+types.GossipDisparityIntervals {
		return errAttestationTooFarInFuture(data.Slot, s.Time())
	}

	return nil
}

// VerifyGossipAttestation enforces validator-id bounds plus XMSS signature
// validity for an incoming individual gossip attestation. Resolves the
// target state from the store, ensures the validator index is within the
// registry, and verifies the signature against the attester's pubkey using
// the xmss.VerifySignatureSSZ primitive the runtime uses for block-included
// attestation signatures.
//
// Pure check — no state mutation. Callers must invoke this before any
// store or fork-choice updates so a rejected attestation leaves no
// observable side effects.
//
// Flow: validate data → resolve target state → bounds-check vid → load
// pubkey → verify signature.
func VerifyGossipAttestation(s *store.ConsensusStore, validatorID uint64, attData *types.AttestationData, dataRoot [32]byte, signature []byte) error {
	targetState := s.GetState(attData.Target.Root)
	if targetState == nil {
		return fmt.Errorf("target state not found in store: 0x%x", attData.Target.Root)
	}
	if validatorID >= uint64(len(targetState.Validators)) {
		return fmt.Errorf("validator %d not found in state (registry size %d)",
			validatorID, len(targetState.Validators))
	}
	if len(signature) != types.SignatureSize {
		return fmt.Errorf("signature length %d != expected %d", len(signature), types.SignatureSize)
	}
	var sig [types.SignatureSize]byte
	copy(sig[:], signature)
	valid, err := xmss.VerifySignatureSSZ(
		targetState.Validators[validatorID].AttestationPubkey,
		uint32(attData.Slot),
		dataRoot,
		sig,
	)
	if err != nil {
		return fmt.Errorf("signature verification error: %w", err)
	}
	if !valid {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// VerifyAggregatedGossipAttestation enforces participant bounds and
// aggregated XMSS proof validity for an incoming gossipAggregatedAttestation.
// Decodes the participants bitlist and delegates to verifyAggregatedProof
// (the same primitive the runtime uses for block-included aggregated
// attestations), which both bounds-checks each participant id and verifies
// the aggregated signature against the participants' pubkeys.
//
// Pure check — no state mutation. Callers must invoke this before any
// store or fork-choice updates.
func VerifyAggregatedGossipAttestation(s *store.ConsensusStore, attData *types.AttestationData, participants []byte, proofData []byte) error {
	targetState := s.GetState(attData.Target.Root)
	if targetState == nil {
		return fmt.Errorf("target state not found in store: 0x%x", attData.Target.Root)
	}
	participantIDs := types.BitlistIndices(participants)
	return verifyAggregatedProof(targetState, participantIDs, attData, proofData)
}

func verifyAggregatedProof(
	state *types.State,
	participantIDs []uint64,
	data *types.AttestationData,
	proofData []byte,
) error {
	numValidators := uint64(len(state.Validators))
	parsedPubkeys := make([]xmss.CPubKey, len(participantIDs))
	for i, vid := range participantIDs {
		if vid >= numValidators {
			return fmt.Errorf("validator %d out of range (%d)", vid, numValidators)
		}
		pk, err := xmss.ParsePublicKey(state.Validators[vid].AttestationPubkey)
		if err != nil {
			for j := range i {
				xmss.FreePublicKey(parsedPubkeys[j])
			}
			return fmt.Errorf("parse pubkey for validator %d: %w", vid, err)
		}
		parsedPubkeys[i] = pk
	}
	defer func() {
		for _, pk := range parsedPubkeys {
			xmss.FreePublicKey(pk)
		}
	}()

	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash tree root: %w", err)
	}
	return xmss.VerifyAggregatedSignature(proofData, parsedPubkeys, dataRoot, uint32(data.Slot))
}
