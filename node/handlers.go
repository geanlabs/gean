// handlers.go contains inbound network message handlers for blocks and attestations.
package node

import (
	"context"
	"errors"
	"fmt"

	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// handleBlock processes an incoming block from the network.
func (n *Node) handleBlock(ctx context.Context, signed *types.SignedBlockWithAttestation, from peer.ID) error {
	block := &signed.Message.Block

	// First, check if we need to request missing parent blocks
	if err := n.syncer.OnBlockReceived(signed, from); err != nil {
		n.logger.Warn("failed to request parent blocks", "error", err)
	}

	// Try to process the block
	if err := n.store.ProcessBlock(block); err != nil {
		// If parent not found, it might be due to missing parent blocks (sync in progress)
		if errors.Is(err, forkchoice.ErrParentNotFound) {
			return fmt.Errorf("%w: %v", ErrSyncInProgress, err)
		}
		return fmt.Errorf("process block: %w", err)
	}

	// Process proposer attestation as a pending (gossip-stage) attestation.
	// This happens after head update inside ProcessBlock to avoid circular weight.
	proposerSigned := &types.SignedAttestation{
		Message: signed.Message.ProposerAttestation,
	}
	if err := n.store.ProcessAttestation(proposerSigned); err != nil {
		n.logger.Warn("failed to process proposer attestation from block envelope",
			"slot", block.Slot,
			"validator", proposerSigned.Message.ValidatorID,
			"error", err,
		)
	}

	// Track that we've seen a block for this slot to prevent proposing for same slot
	if block.Slot > n.lastProposedSlot {
		n.lastProposedSlot = block.Slot
	}

	n.logger.Info("processed block",
		"slot", block.Slot,
		"proposer", block.ProposerIndex,
	)
	return nil
}

// handleAttestation processes an incoming attestation from the network.
func (n *Node) handleAttestation(ctx context.Context, att *types.SignedAttestation) error {
	if err := n.store.ProcessAttestation(att); err != nil {
		return fmt.Errorf("process attestation: %w", err)
	}
	n.logger.Debug("processed attestation",
		"slot", att.Message.Data.Slot,
		"validator", att.Message.ValidatorID,
	)
	return nil
}
