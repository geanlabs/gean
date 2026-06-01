package blockprocessor

import (
	"fmt"
	"runtime"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"golang.org/x/sync/errgroup"
)

// OnBlock processes a new signed block with signature verification.
func OnBlock(
	s *store.ConsensusStore,
	signedBlock *types.SignedBlock,
) error {
	return onBlockCore(s, signedBlock, true)
}

// OnBlockWithoutVerification processes a block without signature checks.
// Used for fork choice spec tests where signatures are absent.
func OnBlockWithoutVerification(
	s *store.ConsensusStore,
	signedBlock *types.SignedBlock,
) error {
	return onBlockCore(s, signedBlock, false)
}

// onBlockCore is the core block processing logic.
func onBlockCore(
	s *store.ConsensusStore,
	signedBlock *types.SignedBlock,
	verify bool,
) error {
	start := time.Now()
	block := signedBlock.Block
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
		return &store.StoreError{Kind: store.ErrMissingParentState,
			Message: fmt.Sprintf("parent state not found for slot %d, missing block %x", slot, block.ParentRoot)}
	}

	// Verify signatures BEFORE state transition.
	// Uses parent_state for validator lookup.
	if verify {
		verifyStart := time.Now()
		err := verifyBlockSignatures(s, signedBlock, parentState)
		metrics.ObserveBlockSignatureVerificationTime(time.Since(verifyStart).Seconds())
		if err != nil {
			return err
		}
	}

	// Enforce unique AttestationData per block + MAX_ATTESTATIONS_DATA cap.
	if block.Body != nil {
		seen := make(map[[32]byte]bool)
		for _, att := range block.Body.Attestations {
			dataRoot, _ := att.Data.HashTreeRoot()
			if seen[dataRoot] {
				return &store.StoreError{Kind: store.ErrDuplicateAttestationData, Message: "block contains duplicate AttestationData"}
			}
			seen[dataRoot] = true
		}
		if len(seen) > int(types.MaxAttestationsData) {
			return &store.StoreError{Kind: store.ErrTooManyAttestationData,
				Message: fmt.Sprintf("block has %d distinct AttestationData (max %d)", len(seen), types.MaxAttestationsData)}
		}
	}

	// Clone state for transition.
	stateBytes, _ := parentState.MarshalSSZ()
	postState := &types.State{}
	postState.UnmarshalSSZ(stateBytes)

	// Execute state transition.
	stfStart := time.Now()
	if err := statetransition.StateTransition(postState, block); err != nil {
		return &store.StoreError{Kind: store.ErrStateTransitionFailed, Message: fmt.Sprintf("state transition: %v", err)}
	}
	metrics.ObserveSTFTime(time.Since(stfStart).Seconds())

	// Cache state root in latest block header.
	postState.LatestBlockHeader.StateRoot = block.StateRoot

	// Check if justified/finalized advanced (strict slot comparison).
	// First root at a given slot wins — no same-slot tiebreak.
	// drainPendingBlocks ensures all nodes process blocks before attesting,
	// so the first-seen root is consistent across nodes.
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
		metrics.IncFinalization("success")
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
	processBlockAttestations(s, signedBlock)

	attCount := 0
	if block.Body != nil {
		attCount = len(block.Body.Attestations)
	}
	elapsed := time.Since(start)
	logger.Info(logger.Chain, "block slot=%d block_root=0x%x parent_root=0x%x proposer=%d attestations=%d justified_slot=%d finalized_slot=%d proc_time=%s",
		slot, blockRoot, block.ParentRoot, block.ProposerIndex, attCount,
		s.LatestJustified().Slot, s.LatestFinalized().Slot,
		elapsed.Round(time.Millisecond))
	metrics.ObserveBlockProcessingTime(elapsed.Seconds())

	return nil
}

// verifyBlockSignatures verifies proposer and attestation signatures.
func verifyBlockSignatures(
	s *store.ConsensusStore,
	signedBlock *types.SignedBlock,
	state *types.State,
) error {
	block := signedBlock.Block
	sigs := signedBlock.Signature

	// Verify proposer signature using the PROPOSAL key.
	// Proposer signs hash_tree_root(block) with proposal key.

	if block.ProposerIndex >= uint64(len(state.Validators)) {
		return &store.StoreError{Kind: store.ErrInvalidValidatorIndex, Message: "proposer index out of range"}
	}
	proposerPubkey := state.Validators[block.ProposerIndex].ProposalPubkey

	blockRoot, _ := block.HashTreeRoot()
	slot := uint32(block.Slot)

	valid, err := xmss.VerifySignatureSSZ(proposerPubkey, slot, blockRoot, sigs.ProposerSignature)
	if err != nil {
		return &store.StoreError{Kind: store.ErrProposerSignatureDecodingFailed, Message: fmt.Sprintf("proposer sig decode: %v", err)}
	}
	if !valid {
		return &store.StoreError{Kind: store.ErrProposerSignatureVerificationFailed, Message: "proposer signature invalid"}
	}

	// Verify attestation aggregate signatures.
	if block.Body == nil {
		return nil
	}

	// Prepare pass: collect everything needed to verify each attestation into
	// a job list. Pubkeys come from store.ConsensusStore.PubKeyCache so the expensive
	// FFI ParsePublicKey runs at most once per validator across the process
	// lifetime; the cache owns the handle, so there's no free list to unwind
	// on error or to carry across the verify pass. Structural errors (mismatch,
	// invalid validator index, pubkey decode) fail fast here before any
	// verification work begins.
	type verifyJob struct {
		attIdx    int
		proofData []byte
		pubkeys   []xmss.CPubKey
		dataRoot  [32]byte
		slot      uint32
	}

	jobs := make([]verifyJob, 0, len(block.Body.Attestations))
	for i, att := range block.Body.Attestations {
		if i >= len(sigs.AttestationSignatures) {
			return &store.StoreError{Kind: store.ErrAttestationSignatureMismatch,
				Message: fmt.Sprintf("attestation %d has no matching signature", i)}
		}
		proof := sigs.AttestationSignatures[i]

		// During checkpoint sync backfill, target states may not exist for
		// attestations referencing blocks before the checkpoint. Skip
		// verification for these — the block was already validated by the
		// originating node.
		targetState := s.GetState(att.Data.Target.Root)
		if targetState == nil {
			continue
		}

		participantIDs := types.BitlistIndices(proof.Participants)
		pubkeys := make([]xmss.CPubKey, 0, len(participantIDs))
		for _, vid := range participantIDs {
			if vid >= uint64(len(targetState.Validators)) {
				return &store.StoreError{Kind: store.ErrInvalidValidatorIndex, Message: fmt.Sprintf("validator %d out of range", vid)}
			}
			handle, err := s.PubKeyCache.Get(targetState.Validators[vid].AttestationPubkey)
			if err != nil {
				return &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: fmt.Sprintf("validator %d: %v", vid, err)}
			}
			pubkeys = append(pubkeys, handle)
		}

		dataRoot, _ := att.Data.HashTreeRoot()
		jobs = append(jobs, verifyJob{
			attIdx:    i,
			proofData: proof.ProofData,
			pubkeys:   pubkeys,
			dataRoot:  dataRoot,
			slot:      uint32(att.Data.Slot),
		})
	}

	// Verify pass: parallel. errgroup.Group with a limit of GOMAXPROCS(0)
	// (not NumCPU — respects container CPU quotas and GOMAXPROCS env). Each
	// VerifyAggregatedSignature is a ~40–50ms cgo call into the Rust XMSS
	// verifier; thread-safety is guaranteed by a single OnceLock<Bytecode>
	// in rec_aggregation that's written once at init and read-only after.
	// Pubkey handles are owned by s.PubKeyCache and stable across goroutines.
	//
	// Note on cancellation: g.Wait() returns the first non-nil error, but
	// the underlying cgo call can't be cancelled — any verifies already
	// dispatched run to completion. Worst case on a single bad signature
	// at the head of a full 16-attestation block is ~GOMAXPROCS verifies'
	// worth of wasted work before g.Wait returns. Acceptable; the
	// alternative (polling a shared cancel flag between calls) buys
	// nothing because verify is a single cgo call, not a loop.
	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			if err := xmss.VerifyAggregatedSignature(job.proofData, job.pubkeys, job.dataRoot, job.slot); err != nil {
				return &store.StoreError{Kind: store.ErrAggregateVerificationFailed, Message: fmt.Sprintf("attestation %d proof: %v", job.attIdx, err)}
			}
			return nil
		})
	}
	return g.Wait()
}

// storeBlockParts stores block body and full signed block across split tables.
func storeBlockParts(s *store.ConsensusStore, blockRoot [32]byte, signedBlock *types.SignedBlock) {
	store.WriteBlockData(s, blockRoot, signedBlock)
}

// processBlockAttestations extracts attestations from block body into known payloads.
func processBlockAttestations(s *store.ConsensusStore, signedBlock *types.SignedBlock) {
	if signedBlock.Block.Body == nil || signedBlock.Signature == nil {
		return
	}
	for i, att := range signedBlock.Block.Body.Attestations {
		if i >= len(signedBlock.Signature.AttestationSignatures) {
			continue
		}
		proof := signedBlock.Signature.AttestationSignatures[i]
		dataRoot, _ := att.Data.HashTreeRoot()
		s.KnownPayloads.Push(dataRoot, att.Data, proof)
	}
}
