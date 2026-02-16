package node

import (
	"context"
	"log/slog"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/leansig"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
)

// ValidatorDuties handles proposer and attester duties.
type ValidatorDuties struct {
	Indices []uint64
	Keys    map[uint64]*leansig.Keypair
	FC      *forkchoice.Store
	Topics  *gossipsub.Topics
	log     *slog.Logger
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
		blockRoot, _ := envelope.Message.Block.HashTreeRoot()
		if err := gossipsub.PublishBlock(ctx, v.Topics.Block, envelope); err != nil {
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
		sa := v.FC.ProduceAttestation(slot, idx)
		if err := gossipsub.PublishAttestation(ctx, v.Topics.Attestation, sa); err != nil {
			v.log.Error("failed to publish attestation",
				"slot", slot,
				"validator", idx,
				"err", err,
			)
		} else {
			v.log.Debug("published attestation",
				"slot", slot,
				"validator", idx,
				"target_slot", sa.Message.Data.Target.Slot,
			)
		}
	}
}
