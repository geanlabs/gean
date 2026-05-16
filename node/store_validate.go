package node

import (
	"fmt"
	"math"

	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// ValidateAttestationData checks 9 validation branches for incoming attestations.
func ValidateAttestationData(s *ConsensusStore, data *types.AttestationData) error {
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
	// Per leanSpec PR #682, the bound is in intervals, not slots — a whole-slot
	// margin would let an adversary pre-publish next-slot aggregates ahead of
	// any honest validator. The first conjunct guards against uint64 overflow
	// for malicious slot values near MaxUint64.
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
// Mirrors ethlambda's on_gossip_attestation flow at
// crates/blockchain/src/store.rs:285-306: validate data → resolve target
// state → bounds-check vid → load pubkey → verify signature.
func VerifyGossipAttestation(s *ConsensusStore, validatorID uint64, attData *types.AttestationData, dataRoot [32]byte, signature []byte) error {
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
//
// Mirrors ethlambda's on_gossip_aggregated_attestation validation path.
func VerifyAggregatedGossipAttestation(s *ConsensusStore, attData *types.AttestationData, participants []byte, proofData []byte) error {
	targetState := s.GetState(attData.Target.Root)
	if targetState == nil {
		return fmt.Errorf("target state not found in store: 0x%x", attData.Target.Root)
	}
	participantIDs := types.BitlistIndices(participants)
	return verifyAggregatedProof(targetState, participantIDs, attData, proofData)
}
