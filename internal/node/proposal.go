package node

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/blockbuilder"
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) maybePropose(slot, validatorID uint64) {
	if e.Keys == nil {
		return
	}
	if e.Store.HeadSlot() >= slot {
		return
	}
	if e.DutyGate != nil && !e.DutyGate.Decide("block", slot, e.Store.HeadSlot(), e.Store.MaxStoredBlockSlot()) {
		metrics.IncBlocksSkippedLag()
		return
	}

	logger.Info(logger.Validator, "proposing block slot=%d validator=%d", slot, validatorID)

	e.Store.PromoteNewToKnown()
	e.updateHead()

	block, attSigProofs, err := e.produceBlockWithSignatures(slot, validatorID)
	if err != nil {
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return
	}

	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Validator, "block root failed: %v", err)
		return
	}
	blockSig, err := propKey.Sign(uint32(slot), blockRoot)
	metrics.ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign block failed: %v", err)
		return
	}

	signedBlock := &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     blockSig,
			AttestationSignatures: attSigProofs,
		},
	}

	if err := blockprocessor.OnBlock(e.Store, signedBlock); err != nil {
		logger.Error(logger.Chain, "local block processing failed: %v", err)
		return
	}

	e.FC.OnBlock(slot, blockRoot, block.ParentRoot)
	e.updateHead()

	if e.P2P != nil {
		if err := e.P2P.PublishBlock(context.Background(), signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	attestationCount := 0
	if block.Body != nil {
		attestationCount = len(block.Body.Attestations)
	}
	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		slot, blockRoot, attestationCount)
}

func (e *Engine) produceBlockWithSignatures(slot, validatorIndex uint64) (*types.Block, []*types.AggregatedSignatureProof, error) {
	buildStart := time.Now()
	defer func() { metrics.ObserveBlockBuildingTime(time.Since(buildStart).Seconds()) }()

	headRoot := e.Store.Head()
	headState := e.Store.GetState(headRoot)
	if headState == nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("head state missing for slot %d", slot)
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("validator %d not proposer for slot %d", validatorIndex, slot)
	}

	knownBlockRoots, err := e.Store.BlockRoots()
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("load block roots: %w", err)
	}

	result, err := blockbuilder.Build(blockbuilder.Input{
		HeadState:         headState,
		Slot:              slot,
		ProposerIndex:     validatorIndex,
		ParentRoot:        headRoot,
		KnownBlockRoots:   knownBlockRoots,
		Payloads:          payloadsFromEntries(e.Store.KnownPayloads.Entries()),
		RequiredJustified: e.Store.LatestJustified(),
		ProofMerger:       attestationproof.NewMerger(e.Store.PubKeyCache),
	})
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, err
	}
	for _, payloadErr := range result.PayloadErrors {
		if blockbuilder.IsExpectedSkip(payloadErr.Err) {
			continue
		}
		logger.Warn(logger.Validator, "block payload issue root=0x%x: %v", payloadErr.DataRoot, payloadErr.Err)
	}

	metrics.IncBlockBuildingSuccess()
	if result.Block != nil && result.Block.Body != nil {
		metrics.ObserveBlockAggregatedPayloads(len(result.Block.Body.Attestations))
	}
	return result.Block, result.AttestationProofs, nil
}

func payloadsFromEntries(entries map[[32]byte]*store.PayloadEntry) []blockbuilder.AttestationPayload {
	payloads := make([]blockbuilder.AttestationPayload, 0, len(entries))
	for dataRoot, entry := range entries {
		payload := blockbuilder.AttestationPayload{DataRoot: dataRoot}
		if entry != nil {
			payload.Data = entry.Data
			payload.Proofs = make([]*types.AggregatedSignatureProof, len(entry.Proofs))
			copy(payload.Proofs, entry.Proofs)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}
