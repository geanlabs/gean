package node

import (
	"context"
	"log/slog"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

// ValidatorDuties handles proposer and attester duties.
type ValidatorDuties struct {
	Indices []uint64
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
		block, err := v.FC.ProduceBlock(slot, idx)
		if err != nil {
			v.log.Error("block proposal failed",
				"slot", slot,
				"proposer", idx,
				"err", err,
			)
			continue
		}
		blockRoot, _ := block.HashTreeRoot()
		sb := &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		}
		if err := gossipsub.PublishBlock(ctx, v.Topics.Block, sb); err != nil {
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
		att := v.FC.ProduceAttestation(slot, idx)
		sa := &types.SignedAttestation{Message: att}
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
				"target_slot", att.Data.Target.Slot,
			)
		}
	}
}
