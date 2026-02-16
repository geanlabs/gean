package node

import (
	"context"
	"log/slog"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

// Signer abstracts the signing capability (XMSS or mock).
type Signer interface {
	Sign(epoch uint32, message [32]byte) ([]byte, error)
}

// ValidatorDuties handles proposer and attester duties.
type ValidatorDuties struct {
	Indices            []uint64
	Keys               map[uint64]Signer
	FC                 *forkchoice.Store
	Topics             *gossipsub.Topics
	PublishBlock       func(context.Context, *pubsub.Topic, *types.SignedBlockWithAttestation) error
	PublishAttestation func(context.Context, *pubsub.Topic, *types.SignedAttestation) error
	log                *slog.Logger
}

// HasProposal reports whether this node has a proposer for the slot.
func (v *ValidatorDuties) HasProposal(slot uint64) bool {
	for _, idx := range v.Indices {
		if statetransition.IsProposer(idx, slot, v.FC.NumValidators) {
			return true
		}
	}
	return false
}

// OnInterval executes validator duties for the current interval.
func (v *ValidatorDuties) OnInterval(ctx context.Context, slot, interval uint64) {
	switch interval {
	case 0:
		v.tryPropose(ctx, slot)
	case 1:
		v.tryAttest(ctx, slot)
	}
}

func (v *ValidatorDuties) tryPropose(ctx context.Context, slot uint64) {
	for _, idx := range v.Indices {
		if !statetransition.IsProposer(idx, slot, v.FC.NumValidators) {
			continue
		}

		envelope, err := v.FC.ProduceBlock(slot, idx)
		if err != nil {
			v.log.Error("block proposal failed",
				"slot", slot,
				"proposer", idx,
				"err", err,
			)
			continue
		}

		// Sign the block.
		kp, ok := v.Keys[idx]
		if !ok {
			v.log.Error("proposer key not found", "validator", idx)
			continue
		}

		// Calculate root of the unsigned message (BlockWithAttestation).
		blockRoot, err := envelope.Message.HashTreeRoot()
		if err != nil {
			v.log.Error("failed to compute block root", "err", err)
			continue
		}

		// Use the current epoch for proposal signature.
		epoch := uint32(slot / types.SlotsPerEpoch)
		sig, err := kp.Sign(epoch, blockRoot)
		if err != nil {
			v.log.Error("failed to sign block", "err", err)
			continue
		}

		// Place signature in the last slot (reserved for proposer).
		lastIdx := len(envelope.Signature) - 1
		copy(envelope.Signature[lastIdx][:], sig[:])

		if err := v.PublishBlock(ctx, v.Topics.Block, envelope); err != nil {
			v.log.Error("failed to publish block",
				"slot", slot,
				"proposer", idx,
				"err", err,
			)
		} else {
			v.log.Info("proposed block",
				"slot", slot,
				"proposer", idx,
				"block_root", logging.ShortHash(blockRoot),
			)
		}
	}
}

func (v *ValidatorDuties) tryAttest(ctx context.Context, slot uint64) {
	for _, idx := range v.Indices {
		// Skip if this validator is the proposer for this slot.
		// The proposer already attests via ProposerAttestation in its block.
		if statetransition.IsProposer(idx, slot, v.FC.NumValidators) {
			continue
		}

		att := v.FC.ProduceAttestation(slot, idx)

		// Sign the attestation.
		kp, ok := v.Keys[idx]
		if !ok {
			v.log.Error("validator key not found", "validator", idx)
			continue
		}

		// Spec: sign(HashTreeRoot(AttestationData))
		dataRoot, err := att.Data.HashTreeRoot()
		if err != nil {
			v.log.Error("failed to compute attestation data root", "err", err)
			continue
		}

		// Use the target epoch for the signature.
		epoch := uint32(att.Data.Target.Slot / types.SlotsPerEpoch)
		sig, err := kp.Sign(epoch, dataRoot)
		if err != nil {
			v.log.Error("failed to sign attestation", "err", err)
			continue
		}

		var sigBytes [3116]byte
		copy(sigBytes[:], sig[:])

		sa := &types.SignedAttestation{
			Message:   att,
			Signature: sigBytes,
		}

		if err := v.PublishAttestation(ctx, v.Topics.Attestation, sa); err != nil {
			v.log.Error("failed to publish attestation",
				"slot", slot,
				"validator", idx,
				"err", err,
			)
		} else {
			v.log.Debug("published attestation",
				"slot", slot,
				"validator", idx,
				"target_slot", att.Data.Target.Slot,
			)
		}
	}
}
