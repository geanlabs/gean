package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// maybePropose builds and publishes a block if we're the proposer.
// Uses store.ProduceBlockWithSignatures for greedy attestation selection.
func (e *Engine) maybePropose(slot, validatorID uint64) {
	if e.Keys == nil {
		return
	}

	// Skip if head is already at this slot (another proposer's block was imported).
	if e.Store.HeadSlot() >= slot {
		return
	}

	logger.Info(logger.Validator, "proposing block slot=%d validator=%d", slot, validatorID)

	// Spec get_proposal_head: promote pending attestations and update head
	// immediately before building. Matches leanSpec get_proposal_head which
	// calls accept_new_attestations (promote + updateHead) before reading head.
	e.Store.PromoteNewToKnown()
	e.updateHead(false)

	// Build block with greedy attestation selection.
	block, attSigProofs, err := ProduceBlockWithSignatures(e.Store, slot, validatorID)
	if err != nil {
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return
	}

	// Sign the block root with the PROPOSAL key.
	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return
	}
	blockRoot, _ := block.HashTreeRoot()
	blockSig, err := propKey.Sign(uint32(slot), blockRoot)
	ObservePqSigSigningTime(time.Since(signStart).Seconds())
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

	// Process locally first.
	if err := OnBlock(e.Store, signedBlock, e.Keys.ValidatorIDs()); err != nil {
		logger.Error(logger.Chain, "local block processing failed: %v", err)
		return
	}

	// Register in fork choice.
	bRoot, _ := block.HashTreeRoot()
	e.FC.OnBlock(slot, bRoot, block.ParentRoot)
	e.updateHead(false)

	// Publish to network.
	if e.P2P != nil {
		if err := e.P2P.PublishBlock(context.Background(), signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		slot, bRoot, len(block.Body.Attestations))
}

// produceAttestations creates and publishes attestations for all local validators.
func (e *Engine) produceAttestations(slot uint64) {
	if e.Keys == nil {
		return
	}

	attData := ProduceAttestationData(e.Store, slot)
	if attData == nil {
		return
	}

	for _, vid := range e.Keys.ValidatorIDs() {
		sStart := time.Now()
		sig, err := e.Keys.SignAttestation(vid, attData)
		ObservePqSigSigningTime(time.Since(sStart).Seconds())
		if err != nil {
			logger.Error(logger.Validator, "sign attestation failed validator=%d: %v", vid, err)
			continue
		}

		signedAtt := &types.SignedAttestation{
			ValidatorID: vid,
			Data:        attData,
			Signature:   sig,
		}

		logger.Info(logger.Validator, "produced attestation slot=%d validator=%d", slot, vid)

		// Self-deliver for aggregation if we are the aggregator.
		// Skip signature verification — we just signed it ourselves.
		if e.AggCtl.Get() {
			dataRoot, _ := attData.HashTreeRoot()
			sigHandle, parseErr := xmss.ParseSignature(sig[:])
			e.Store.AttestationSignatures.InsertWithHandle(dataRoot, attData, vid, sig, sigHandle, parseErr)
		}

		// Publish to subnet.
		if e.P2P != nil {
			if err := e.P2P.PublishAttestation(context.Background(), signedAtt, e.CommitteeCount); err != nil {
				logger.Error(logger.Network, "publish attestation failed validator=%d: %v", vid, err)
			} else {
				logger.Info(logger.Network, "published attestation to network slot=%d validator=%d", slot, vid)
			}
		}
	}
}
