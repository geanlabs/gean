package node

import (
	"context"
	"log/slog"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
)

// processBlockWorker drains the block channel and processes blocks sequentially.
// Runs in its own goroutine so gossip readers are never blocked by slow
// state transitions, signature verification, or parent sync recovery.
func (n *Node) processBlockWorker(ctx context.Context) {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)
	for {
		select {
		case <-ctx.Done():
			return
		case sb := <-n.blockCh:
			n.handleBlock(sb, gossipLog)
		}
	}
}

// handleBlock contains the block processing logic previously inline in the
// gossip handler callback. Extracted so it can run in the worker goroutine.
func (n *Node) handleBlock(sb *types.SignedBlockWithAttestation, gossipLog *slog.Logger) {
	block := sb.Message.Block
	blockRoot, _ := block.HashTreeRoot()
	gossipLog.Info("received block via gossip",
		"slot", block.Slot,
		"proposer", block.ProposerIndex,
		"block_root", logging.LongHash(blockRoot),
		"parent_root", logging.LongHash(block.ParentRoot),
		"state_root", logging.LongHash(block.StateRoot),
		"attestations", len(block.Body.Attestations),
	)
	if err := n.FC.ProcessBlock(sb); err != nil {
		status := n.FC.GetStatus()
		if isMissingParentStateErr(err) {
			gossipLog.Warn("parent state missing for gossip block, attempting recovery",
				"slot", block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"parent_root", logging.LongHash(block.ParentRoot),
				"head_slot", status.HeadSlot,
				"finalized_slot", status.FinalizedSlot,
			)
			if n.recoverMissingParentSync(n.Host.Ctx, block.ParentRoot) {
				if retryErr := n.FC.ProcessBlock(sb); retryErr == nil {
					gossipLog.Info("accepted gossip block after parent recovery",
						"slot", block.Slot,
						"block_root", logging.LongHash(blockRoot),
					)
					n.processPendingChildren(blockRoot, gossipLog)
					return
				} else {
					err = retryErr
				}
			}
			n.PendingBlocks.Add(sb)
			gossipLog.Info("cached pending block awaiting parent",
				"slot", block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"parent_root", logging.LongHash(block.ParentRoot),
				"pending_count", n.PendingBlocks.Len(),
			)
			return
		}
		gossipLog.Warn("rejected gossip block",
			"slot", block.Slot,
			"block_root", logging.LongHash(blockRoot),
			"err", err,
			"head_slot", status.HeadSlot,
			"finalized_slot", status.FinalizedSlot,
		)
		return
	}
	gossipLog.Info("block accepted",
		"slot", block.Slot,
		"proposer", block.ProposerIndex,
		"block_root", logging.LongHash(blockRoot),
		"parent_root", logging.LongHash(block.ParentRoot),
		"state_root", logging.LongHash(block.StateRoot),
		"attestations", len(block.Body.Attestations),
	)
	n.processPendingChildren(blockRoot, gossipLog)
}

// processAttestationWorker drains the attestation channel.
func (n *Node) processAttestationWorker(ctx context.Context) {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)
	for {
		select {
		case <-ctx.Done():
			return
		case sa := <-n.attestationCh:
			if sa.Message != nil {
				gossipLog.Debug("received attestation from gossip",
					"slot", sa.Message.Slot,
					"validator", sa.ValidatorID,
					"head_root", logging.LongHash(sa.Message.Head.Root),
					"target_slot", sa.Message.Target.Slot,
					"target_root", logging.LongHash(sa.Message.Target.Root),
					"source_slot", sa.Message.Source.Slot,
					"source_root", logging.LongHash(sa.Message.Source.Root),
				)
			}
			n.FC.ProcessSubnetAttestation(sa)
		}
	}
}

// processAggregationWorker drains the aggregated attestation channel.
func (n *Node) processAggregationWorker(ctx context.Context) {
	gossipLog := logging.NewComponentLogger(logging.CompGossip)
	for {
		select {
		case <-ctx.Done():
			return
		case saa := <-n.aggregationCh:
			gossipLog.Debug("received aggregated attestation via gossip",
				"slot", saa.Data.Slot,
			)
			n.FC.ProcessAggregatedAttestation(saa)
		}
	}
}
