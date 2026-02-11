package networking

import (
	"context"
	"fmt"

	gossipcfg "github.com/devylongs/gean/networking/gossipsub"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// BlockHandler processes incoming blocks from gossipsub.
type BlockHandler func(ctx context.Context, block *types.SignedBlockWithAttestation, from peer.ID) error

// AttestationHandler processes incoming attestations from gossipsub.
type AttestationHandler func(ctx context.Context, att *types.SignedAttestation) error

// MessageHandlers holds handlers for different message types.
type MessageHandlers struct {
	OnBlock       BlockHandler
	OnAttestation AttestationHandler
}

// HandleBlockMessage decodes and processes an incoming block message.
func (h *MessageHandlers) HandleBlockMessage(ctx context.Context, data []byte, from peer.ID) error {
	decoded, err := gossipcfg.DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress block: %w", err)
	}

	var block types.SignedBlockWithAttestation
	if err := block.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal block: %w", err)
	}

	if h.OnBlock != nil {
		return h.OnBlock(ctx, &block, from)
	}
	return nil
}

// HandleAttestationMessage decodes and processes an incoming attestation message.
func (h *MessageHandlers) HandleAttestationMessage(ctx context.Context, data []byte) error {
	decoded, err := gossipcfg.DecompressMessage(data)
	if err != nil {
		return fmt.Errorf("decompress attestation: %w", err)
	}

	var att types.SignedAttestation
	if err := att.UnmarshalSSZ(decoded); err != nil {
		return fmt.Errorf("unmarshal attestation: %w", err)
	}

	if h.OnAttestation != nil {
		return h.OnAttestation(ctx, &att)
	}
	return nil
}
