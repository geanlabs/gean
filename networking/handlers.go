package networking

import (
	"context"
	"fmt"

	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// BlockHandler processes incoming blocks from gossipsub.
type BlockHandler func(ctx context.Context, block *types.SignedBlock, from peer.ID) error

// VoteHandler processes incoming votes from gossipsub.
type VoteHandler func(ctx context.Context, vote *types.SignedVote) error

// MessageHandlers holds handlers for different message types.
type MessageHandlers struct {
	OnBlock BlockHandler
	OnVote  VoteHandler
}

// HandleBlockMessage decodes and processes an incoming block message.
func (h *MessageHandlers) HandleBlockMessage(ctx context.Context, data []byte, from peer.ID) error {
	decoded, err := DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress block: %w", err)
	}

	var block types.SignedBlock
	if err := block.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal block: %w", err)
	}

	if h.OnBlock != nil {
		return h.OnBlock(ctx, &block, from)
	}
	return nil
}

// HandleVoteMessage decodes and processes an incoming vote.
func (h *MessageHandlers) HandleVoteMessage(ctx context.Context, data []byte) error {
	decoded, err := DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress vote: %w", err)
	}

	var vote types.SignedVote
	if err := vote.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal vote: %w", err)
	}

	if h.OnVote != nil {
		return h.OnVote(ctx, &vote)
	}
	return nil
}
