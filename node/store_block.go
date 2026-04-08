package node

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// OnBlock processes a new signed block with signature verification.
func OnBlock(
	s *ConsensusStore,
	signedBlock *types.SignedBlockWithAttestation,
	localValidatorIDs []uint64,
) error {
	return onBlockCore(s, signedBlock, true, localValidatorIDs)
}

// OnBlockWithoutVerification processes a block without signature checks.
// Used for fork choice spec tests where signatures are absent.
// Caller must call ProcessProposerAttestation(s, signedBlock, false) AFTER updateHead.
func OnBlockWithoutVerification(
	s *ConsensusStore,
	signedBlock *types.SignedBlockWithAttestation,
) error {
	return onBlockCore(s, signedBlock, false, nil)
}

// onBlockCore is the core block processing logic.
func onBlockCore(
	s *ConsensusStore,
	signedBlock *types.SignedBlockWithAttestation,
	verify bool,
	localValidatorIDs []uint64,
) error {
	start := time.Now()
	block := signedBlock.Block.Block
	slot := block.Slot

	// Compute block root.
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}

	// Skip duplicate blocks.
	if s.HasState(blockRoot) {
		return nil // already known
	}

	// Get parent state.
	parentState := s.GetState(block.ParentRoot)
	if parentState == nil {
		return &StoreError{ErrMissingParentState,
			fmt.Sprintf("parent state not found for slot %d, missing block %x", slot, block.ParentRoot)}
	}

	// Verify signatures BEFORE state transition.
	// Uses parent_state for validator lookup.
	if verify {
		if err := verifyBlockSignatures(s, signedBlock, parentState); err != nil {
			return err
		}
	}

	// Clone state for transition.
	stateBytes, _ := parentState.MarshalSSZ()
	postState := &types.State{}
	postState.UnmarshalSSZ(stateBytes)

	// Execute state transition.
	if err := statetransition.StateTransition(postState, block); err != nil {
		return &StoreError{ErrStateTransitionFailed, fmt.Sprintf("state transition: %v", err)}
	}

	// Cache state root in latest block header.
	postState.LatestBlockHeader.StateRoot = block.StateRoot

	// Check if justified/finalized advanced.
	var newJustified, newFinalized *types.Checkpoint
	currentJustified := s.LatestJustified()
	currentFinalized := s.LatestFinalized()

	if postState.LatestJustified.Slot > currentJustified.Slot {
		newJustified = postState.LatestJustified
	}
	if postState.LatestFinalized.Slot > currentFinalized.Slot {
		newFinalized = postState.LatestFinalized
	}

	// Update checkpoints.
	if newJustified != nil {
		s.SetLatestJustified(newJustified)
	}
	if newFinalized != nil {
		s.SetLatestFinalized(newFinalized)
	}

	// Store block header, state, and live chain entry.
	header := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
	}
	bodyRoot, _ := block.Body.HashTreeRoot()
	header.BodyRoot = bodyRoot

	s.InsertBlockHeader(blockRoot, header)
	s.InsertState(blockRoot, postState)
	s.InsertLiveChainEntry(slot, blockRoot, block.ParentRoot)

	// Store block body and signatures.
	storeBlockParts(s, blockRoot, signedBlock)

	// Process block body attestations into known payloads.
	processBlockAttestations(s, signedBlock, blockRoot)

	// NOTE: Proposer attestation is NOT processed here.
	// Engine must call updateHead BEFORE ProcessProposerAttestation
	// to prevent circular weight advantage.

	attCount := 0
	if block.Body != nil {
		attCount = len(block.Body.Attestations)
	}
	logger.Info(logger.Chain, "block slot=%d block_root=0x%x parent_root=0x%x proposer=%d attestations=%d justified_slot=%d finalized_slot=%d proc_time=%s",
		slot, blockRoot, block.ParentRoot, block.ProposerIndex, attCount,
		s.LatestJustified().Slot, s.LatestFinalized().Slot,
		time.Since(start).Round(time.Millisecond))

	return nil
}

// ProcessProposerAttestation processes the proposer's self-attestation.
// Must be called AFTER updateHead to prevent circular weight advantage.
func ProcessProposerAttestation(s *ConsensusStore, signedBlock *types.SignedBlockWithAttestation, verify bool) {
	if signedBlock.Block.ProposerAttestation == nil {
		return
	}
	blockRoot, _ := signedBlock.Block.Block.HashTreeRoot()
	processProposerAttestation(s, signedBlock, blockRoot, verify)
}

// verifyBlockSignatures verifies proposer and attestation signatures.
func verifyBlockSignatures(
	s *ConsensusStore,
	signedBlock *types.SignedBlockWithAttestation,
	state *types.State,
) error {
	block := signedBlock.Block.Block
	sigs := signedBlock.Signature

	// Verify proposer attestation signature.
	// ProposerSignature signs the proposer's AttestationData, NOT the block root.

	if block.ProposerIndex >= uint64(len(state.Validators)) {
		return &StoreError{ErrInvalidValidatorIndex, "proposer index out of range"}
	}
	proposerPubkey := state.Validators[block.ProposerIndex].Pubkey

	proposerAtt := signedBlock.Block.ProposerAttestation
	if proposerAtt == nil || proposerAtt.Data == nil {
		return &StoreError{ErrProposerSignatureVerificationFailed, "missing proposer attestation data"}
	}
	attDataRoot, _ := proposerAtt.Data.HashTreeRoot()
	slot := uint32(proposerAtt.Data.Slot)

	valid, err := xmss.VerifySignatureSSZ(proposerPubkey, slot, attDataRoot, sigs.ProposerSignature)
	if err != nil {
		return &StoreError{ErrProposerSignatureDecodingFailed, fmt.Sprintf("proposer sig decode: %v", err)}
	}
	if !valid {
		return &StoreError{ErrProposerSignatureVerificationFailed, "proposer signature invalid"}
	}

	// Verify attestation aggregate signatures.
	if block.Body == nil {
		return nil
	}
	for i, att := range block.Body.Attestations {
		if i >= len(sigs.AttestationSignatures) {
			return &StoreError{ErrAttestationSignatureMismatch,
				fmt.Sprintf("attestation %d has no matching signature", i)}
		}
		proof := sigs.AttestationSignatures[i]

		// Get participant pubkeys.
		// During checkpoint sync backfill, target states may not exist for
		// attestations referencing blocks before the checkpoint. Skip verification
		// for these — the block was already validated by the originating node.
		targetState := s.GetState(att.Data.Target.Root)
		if targetState == nil {
			continue // skip attestation verification when target state unavailable
		}

		participantIDs := types.BitlistIndices(proof.Participants)
		var pubkeys []([types.PubkeySize]byte)
		for _, vid := range participantIDs {
			if vid >= uint64(len(targetState.Validators)) {
				return &StoreError{ErrInvalidValidatorIndex, fmt.Sprintf("validator %d out of range", vid)}
			}
			pubkeys = append(pubkeys, targetState.Validators[vid].Pubkey)
		}

		// Verify aggregated proof.
		dataRoot, _ := att.Data.HashTreeRoot()
		attSlot := uint32(att.Data.Slot)

		parsedPubkeys := make([]xmss.CPubKey, len(pubkeys))
		for j, pk := range pubkeys {
			parsed, err := xmss.ParsePublicKey(pk)
			if err != nil {
				// Free already parsed keys before returning.
				for k := 0; k < j; k++ {
					xmss.FreePublicKey(parsedPubkeys[k])
				}
				return &StoreError{ErrPubkeyDecodingFailed, fmt.Sprintf("pubkey %d: %v", participantIDs[j], err)}
			}
			parsedPubkeys[j] = parsed
		}
		// Free all parsed pubkeys after verification.
		defer func() {
			for _, pk := range parsedPubkeys {
				xmss.FreePublicKey(pk)
			}
		}()

		if err := xmss.VerifyAggregatedSignature(proof.ProofData, parsedPubkeys, dataRoot, attSlot); err != nil {
			return &StoreError{ErrAggregateVerificationFailed, fmt.Sprintf("attestation %d proof: %v", i, err)}
		}
	}

	return nil
}

// storeBlockParts stores block body and full signed block across split tables.
func storeBlockParts(s *ConsensusStore, blockRoot [32]byte, signedBlock *types.SignedBlockWithAttestation) {
	writeBlockData(s, blockRoot, signedBlock)
}

// processBlockAttestations extracts attestations from block body into known payloads.
func processBlockAttestations(s *ConsensusStore, signedBlock *types.SignedBlockWithAttestation, blockRoot [32]byte) {
	if signedBlock.Block.Block.Body == nil || signedBlock.Signature == nil {
		return
	}
	for i, att := range signedBlock.Block.Block.Body.Attestations {
		if i >= len(signedBlock.Signature.AttestationSignatures) {
			continue
		}
		proof := signedBlock.Signature.AttestationSignatures[i]
		dataRoot, _ := att.Data.HashTreeRoot()
		s.KnownPayloads.Push(dataRoot, att.Data, proof)
	}
}

// processProposerAttestation handles the proposer's self-attestation.
// Production (verify=true): store proposer's real XMSS signature in gossip for aggregation at interval 2.
// Spec tests only (verify=false via OnBlockWithoutVerification): insert participants-only proof
// into new payloads since no real signatures exist in test fixtures.
func processProposerAttestation(s *ConsensusStore, signedBlock *types.SignedBlockWithAttestation, blockRoot [32]byte, verify bool) {
	att := signedBlock.Block.ProposerAttestation
	if att == nil || att.Data == nil {
		return
	}
	dataRoot, _ := att.Data.HashTreeRoot()

	if verify && signedBlock.Signature != nil {
		// Store proposer's gossip signature for aggregation with C handle.
		// ParseSignature creates a native leansig handle from SSZ bytes.
		sigHandle, parseErr := xmss.ParseSignature(signedBlock.Signature.ProposerSignature[:])
		s.GossipSignatures.InsertWithHandle(dataRoot, att.Data, att.ValidatorID, signedBlock.Signature.ProposerSignature, sigHandle, parseErr)
	} else {
		// Without sig verification, insert directly with a dummy proof.
		participants := aggregationBitsFromValidatorIndices([]uint64{att.ValidatorID})
		proof := &types.AggregatedSignatureProof{
			Participants: participants,
			ProofData:    nil,
		}
		s.NewPayloads.Push(dataRoot, att.Data, proof)
	}
}
