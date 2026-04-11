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

	// Build block with greedy attestation selection.
	block, attSigProofs, err := ProduceBlockWithSignatures(e.Store, slot, validatorID)
	if err != nil {
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return
	}

	// Produce proposer's own attestation.
	attData := ProduceAttestationData(e.Store, slot)
	if attData == nil {
		logger.Error(logger.Validator, "failed to produce attestation data for proposal")
		return
	}

	proposerAtt := &types.Attestation{
		ValidatorID: validatorID,
		Data:        attData,
	}

	// Sign proposer's attestation with the PROPOSAL key (not attestation key).
	// The proposer uses a separate key domain for block-related signatures.
	// Phase 4 will change the signed message from attestation data to block root.
	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return
	}
	attDataRoot, _ := attData.HashTreeRoot()
	attSig, err := propKey.Sign(uint32(slot), attDataRoot)
	ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign proposer attestation failed: %v", err)
		return
	}

	signedBlock := &types.SignedBlockWithAttestation{
		Block: &types.BlockWithAttestation{
			Block:               block,
			ProposerAttestation: proposerAtt,
		},
		Signature: &types.BlockSignatures{
			ProposerSignature:     attSig, // attestation signature, NOT block signature
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

	// Store proposer's attestation signature in gossip for aggregation with C handle.
	dataRoot, _ := attData.HashTreeRoot()
	sigHandle, parseErr := xmss.ParseSignature(attSig[:])
	e.Store.GossipSignatures.InsertWithHandle(dataRoot, attData, validatorID, attSig, sigHandle, parseErr)

	// Publish to network.
	if e.P2P != nil {
		if err := e.P2P.PublishBlock(context.Background(), signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		slot, bRoot, len(block.Body.Attestations))
}

// produceAttestations creates and publishes attestations for non-proposing validators.
func (e *Engine) produceAttestations(slot uint64) {
	if e.Keys == nil {
		return
	}

	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return
	}
	numValidators := headState.NumValidators()

	attData := ProduceAttestationData(e.Store, slot)
	if attData == nil {
		return
	}

	for _, vid := range e.Keys.ValidatorIDs() {
		// Skip proposer — they already attested via block.
		if types.IsProposer(slot, vid, numValidators) {
			continue
		}

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
