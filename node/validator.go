package node

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/geanlabs/gean/types"
)

// ValidatorDuties handles proposer and attester duties.
type ValidatorDuties struct {
	Indices                      []uint64
	Keys                         map[uint64]forkchoice.Signer
	FC                           *forkchoice.Store
	Topics                       *gossipsub.Topics
	PublishBlock                 func(context.Context, *pubsub.Topic, *types.SignedBlockWithAttestation) error
	PublishAttestation           func(context.Context, *pubsub.Topic, *types.SignedAttestation) error
	PublishAggregatedAttestation func(context.Context, *pubsub.Topic, *types.AggregatedAttestation) error
	Log                          *slog.Logger

	// pendingAttestations collects signed attestations produced during interval 1
	// for aggregation during interval 2.
	pendingAttestations []*types.SignedAttestation
}

// HasProposal reports whether this node has a proposer for the slot.
func (v *ValidatorDuties) HasProposal(slot uint64) bool {
	for _, idx := range v.Indices {
		if statetransition.IsProposer(idx, slot, v.FC.NumValidators()) {
			return true
		}
	}
	return false
}

// OnInterval executes validator duties for the current interval.
func (v *ValidatorDuties) OnInterval(ctx context.Context, slot, interval uint64) {
	switch interval {
	case 0:
		v.TryPropose(ctx, slot)
	case 1:
		v.TryAttest(ctx, slot)
	case 2:
		v.TryAggregate(ctx, slot)
	}
}

func (v *ValidatorDuties) TryPropose(ctx context.Context, slot uint64) {
	// Slot 0 is the anchor/genesis slot and should not produce a new block.
	if slot == 0 {
		return
	}

	for _, idx := range v.Indices {
		if !statetransition.IsProposer(idx, slot, v.FC.NumValidators()) {
			continue
		}

		kp, ok := v.Keys[idx]
		if !ok {
			v.Log.Error("proposer key not found", "validator", idx)
			continue
		}

		envelope, err := v.FC.ProduceBlock(slot, idx, kp)
		if err != nil {
			v.Log.Error("block proposal failed",
				"slot", slot,
				"proposer", idx,
				"err", err,
			)
			continue
		}

		blockRoot, _ := envelope.Message.Block.HashTreeRoot()

		// Log signing confirmation.
		lastIdx := len(envelope.Signature) - 1
		proposerSig := envelope.Signature[lastIdx]
		v.Log.Info("block signed (XMSS)",
			"slot", slot,
			"proposer", idx,
			"sig_size", fmt.Sprintf("%d bytes", len(proposerSig)),
			"sig_prefix", hex.EncodeToString(proposerSig[:8]),
		)

		if err := v.PublishBlock(ctx, v.Topics.Block, envelope); err != nil {
			v.Log.Error("failed to publish block",
				"slot", slot,
				"proposer", idx,
				"err", err,
			)
		} else {
			v.Log.Info("proposed block",
				"slot", slot,
				"proposer", idx,
				"block_root", logging.ShortHash(blockRoot),
			)
		}
	}
}

func (v *ValidatorDuties) TryAttest(ctx context.Context, slot uint64) {
	v.pendingAttestations = nil // reset for this slot

	for _, idx := range v.Indices {
		// Skip if this validator is the proposer for this slot.
		// The proposer already attests via ProposerAttestation in its block.
		if statetransition.IsProposer(idx, slot, v.FC.NumValidators()) {
			continue
		}

		kp, ok := v.Keys[idx]
		if !ok {
			v.Log.Error("validator key not found", "validator", idx)
			continue
		}

		signStart := time.Now()
		sa, err := v.FC.ProduceAttestation(slot, idx, kp)
		signDuration := time.Since(signStart)
		metrics.SigningTime.Observe(signDuration.Seconds())

		if err != nil {
			v.Log.Error("attestation failed",
				"slot", slot,
				"validator", idx,
				"err", err,
			)
			continue
		}

		// Log signing confirmation.
		v.Log.Info("attestation signed (XMSS)",
			"slot", slot,
			"validator", idx,
			"sig_size", fmt.Sprintf("%d bytes", len(sa.Signature)),
			"sig_prefix", hex.EncodeToString(sa.Signature[:8]),
			"signing_time", signDuration,
		)

		v.pendingAttestations = append(v.pendingAttestations, sa)

		// Process locally so the vote counts even without gossip self-delivery.
		v.FC.ProcessAttestation(sa)

		if err := v.PublishAttestation(ctx, v.Topics.Attestation, sa); err != nil {
			v.Log.Error("failed to publish attestation",
				"slot", slot,
				"validator", idx,
				"err", err,
			)
		} else {
			v.Log.Debug("published attestation",
				"slot", slot,
				"validator", idx,
				"target_slot", sa.Message.Data.Target.Slot,
			)
		}
	}
}

// TryAggregate aggregates pending attestations from interval 1 and publishes
// the aggregate to the aggregate_attestation gossip topic.
func (v *ValidatorDuties) TryAggregate(ctx context.Context, slot uint64) {
	if len(v.pendingAttestations) == 0 {
		return
	}

	agg, err := forkchoice.AggregateAttestations(v.pendingAttestations)
	if err != nil {
		v.Log.Error("aggregation failed",
			"slot", slot,
			"num_attestations", len(v.pendingAttestations),
			"err", err,
		)
		return
	}

	aggSize := len(agg.AggregatedSignature)
	metrics.AggregateSizeBytes.Set(float64(aggSize))

	v.Log.Info("aggregated attestations",
		"slot", slot,
		"num_attestations", len(v.pendingAttestations),
		"aggregate_size", fmt.Sprintf("%d bytes", aggSize),
	)

	if v.PublishAggregatedAttestation != nil && v.Topics.AggregateAttestation != nil {
		if err := v.PublishAggregatedAttestation(ctx, v.Topics.AggregateAttestation, agg); err != nil {
			v.Log.Error("failed to publish aggregated attestation",
				"slot", slot,
				"err", err,
			)
		} else {
			v.Log.Debug("published aggregated attestation",
				"slot", slot,
				"num_sigs", len(v.pendingAttestations),
			)
		}
	}

	v.pendingAttestations = nil
}
