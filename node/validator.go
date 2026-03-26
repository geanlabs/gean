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

// ValidatorDuties handles proposer, attester, and aggregator duties.
type ValidatorDuties struct {
	Indices                      []uint64
	Keys                         map[uint64]forkchoice.Signer
	FC                           *forkchoice.Store
	Topics                       *gossipsub.Topics
	PublishBlock                 func(context.Context, *pubsub.Topic, *types.SignedBlockWithAttestation) error
	PublishAttestation           func(context.Context, *pubsub.Topic, *types.SignedAttestation) error
	PublishAggregatedAttestation func(context.Context, *pubsub.Topic, *types.SignedAggregatedAttestation) error
	IsAggregator                 bool
	AttestationCommitteeCount    uint64
	Log                          *slog.Logger
	lastProposedSlot             map[uint64]uint64
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
		if v.IsAggregator {
			v.TryAggregate(ctx, slot)
		}
	}
}

// TryAggregate aggregates collected subnet attestation signatures and publishes
// SignedAggregatedAttestation messages on the aggregation gossip topic.
// Called at interval 2 only by aggregator nodes.
func (v *ValidatorDuties) TryAggregate(ctx context.Context, slot uint64) {
	start := time.Now()
	aggregated, err := v.FC.AggregateCommitteeSignatures()
	if err != nil {
		v.Log.Error("aggregation failed", "slot", slot, "err", err)
		return
	}

	if len(aggregated) == 0 {
		v.Log.Debug("no attestations to aggregate", "slot", slot)
		return
	}

	for _, saa := range aggregated {
		if err := v.PublishAggregatedAttestation(ctx, v.Topics.Aggregation, saa); err != nil {
			v.Log.Error("failed to publish aggregated attestation",
				"slot", slot,
				"err", err,
			)
		} else {
			v.Log.Info("published aggregated attestation",
				"slot", slot,
				"att_slot", saa.Data.Slot,
				"participants", countBitlistParticipants(saa.Proof.Participants),
				"participants_bitlist_bytes", len(saa.Proof.Participants),
				"proof_size", len(saa.Proof.ProofData),
			)
		}
	}
	duration := time.Since(start)
	metrics.CommitteeSignaturesAggregationTime.Observe(duration.Seconds())
	v.Log.Info("aggregation complete",
		"slot", slot,
		"count", len(aggregated),
		"duration", duration,
	)
}

func (v *ValidatorDuties) TryPropose(ctx context.Context, slot uint64) {
	// Slot 0 is the anchor/genesis slot and should not produce a new block.
	if slot == 0 {
		return
	}
	if v.lastProposedSlot == nil {
		v.lastProposedSlot = make(map[uint64]uint64)
	}

	for _, idx := range v.Indices {
		if !statetransition.IsProposer(idx, slot, v.FC.NumValidators()) {
			continue
		}
		if lastSlot, ok := v.lastProposedSlot[idx]; ok && lastSlot == slot {
			continue
		}
		v.lastProposedSlot[idx] = slot

		kp, ok := v.Keys[idx]
		if !ok {
			v.Log.Error("proposer key not found", "validator", idx)
			continue
		}

		envelope, err := v.FC.ProduceBlock(slot, idx, kp)
		if err != nil {
			status := v.FC.GetStatus()
			v.Log.Error("block proposal failed",
				"slot", slot,
				"proposer", idx,
				"err", err,
				"head_slot", status.HeadSlot,
				"finalized_slot", status.FinalizedSlot,
			)
			continue
		}

		blockRoot, _ := envelope.Message.Block.HashTreeRoot()

		// Log signing confirmation.
		proposerSig := envelope.Signature.ProposerSignature
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
				"block_root", logging.LongHash(blockRoot),
				"err", err,
			)
		} else {
			v.Log.Info("proposed block",
				"slot", slot,
				"proposer", idx,
				"block_root", logging.LongHash(blockRoot),
				"parent_root", logging.LongHash(envelope.Message.Block.ParentRoot),
				"state_root", logging.LongHash(envelope.Message.Block.StateRoot),
				"attestations", len(envelope.Message.Block.Body.Attestations),
			)
		}
	}
}

func (v *ValidatorDuties) TryAttest(ctx context.Context, slot uint64) {
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
		metrics.PQSigAttestationSigningTime.Observe(signDuration.Seconds())

		if err != nil {
			status := v.FC.GetStatus()
			v.Log.Error("attestation failed",
				"slot", slot,
				"validator", idx,
				"err", err,
				"head_slot", status.HeadSlot,
				"finalized_slot", status.FinalizedSlot,
			)
			continue
		}

		// Log signing confirmation.
		metrics.PQSigAttestationSignaturesTotal.Inc()
		v.Log.Info("attestation signed (XMSS)",
			"slot", slot,
			"validator", idx,
			"sig_size", fmt.Sprintf("%d bytes", len(sa.Signature)),
			"sig_prefix", hex.EncodeToString(sa.Signature[:8]),
			"signing_time", signDuration,
		)

		// Route to the correct subnet topic for this validator.
		committeeCount := v.AttestationCommitteeCount
		if committeeCount == 0 {
			committeeCount = 1
		}
		subnetID := idx % committeeCount
		subnetTopic := v.Topics.GetSubnetTopic(subnetID)

		// Warn if no peers are subscribed — publish will be silently dropped with no error.
		if topicPeerCount(subnetTopic) == 0 {
			v.Log.Warn("attestation topic has 0 peers — published attestation will not be delivered",
				"slot", slot,
				"validator", idx,
				"subnet_id", subnetID,
			)
		}

		if err := v.PublishAttestation(ctx, subnetTopic, sa); err != nil {
			v.Log.Error("failed to publish attestation",
				"slot", slot,
				"validator", idx,
				"subnet_id", subnetID,
				"err", err,
			)
		} else {
			v.Log.Debug("published attestation",
				"slot", slot,
				"validator", idx,
				"subnet_id", subnetID,
				"head_root", logging.LongHash(sa.Message.Head.Root),
				"target_slot", sa.Message.Target.Slot,
				"target_root", logging.LongHash(sa.Message.Target.Root),
				"source_slot", sa.Message.Source.Slot,
				"source_root", logging.LongHash(sa.Message.Source.Root),
			)
		}
	}
}

func countBitlistParticipants(bits []byte) int {
	numBits := uint64(statetransition.BitlistLen(bits))
	count := 0
	for i := uint64(0); i < numBits; i++ {
		if statetransition.GetBit(bits, i) {
			count++
		}
	}
	return count
}

// topicPeerCount safely returns the number of peers subscribed to a pubsub topic.
// Returns 0 if the topic is nil or has no backing PubSub instance (e.g. in tests).
func topicPeerCount(topic *pubsub.Topic) (n int) {
	if topic == nil {
		return 0
	}
	defer func() { recover() }() //nolint:errcheck
	return len(topic.ListPeers())
}
