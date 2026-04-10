package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
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

	// Sign the block root with the proposer's key.
	blockRoot, _ := block.HashTreeRoot()
	signStart := time.Now()
	proposerSig, err := e.Keys.SignBlock(validatorID, slot, blockRoot)
	ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign block failed: %v", err)
		return
	}

	signedBlock := &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     proposerSig,
			AttestationSignatures: attSigProofs,
		},
	}

	// Process locally first.
	if err := OnBlock(e.Store, signedBlock, e.Keys.ValidatorIDs()); err != nil {
		logger.Error(logger.Chain, "local block processing failed: %v", err)
		return
	}

	// Register in fork choice.
	e.FC.OnBlock(slot, blockRoot, block.ParentRoot)
	e.updateHead(false)

	// Publish to network.
	if e.P2P != nil {
		if err := e.P2P.PublishBlock(context.Background(), signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		slot, blockRoot, len(block.Body.Attestations))
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
