// publish.go contains outbound gossip publish methods.
package networking

import (
	"context"
	"fmt"

	gossipcfg "github.com/devylongs/gean/networking/gossipsub"
	"github.com/devylongs/gean/types"
)

// PublishBlock publishes a signed block to the network.
func (s *Service) PublishBlock(ctx context.Context, block *types.SignedBlockWithAttestation) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	compressed := gossipcfg.CompressMessage(data)
	return s.blockTopic.Publish(ctx, compressed)
}

// PublishAttestation publishes a signed attestation to the network.
func (s *Service) PublishAttestation(ctx context.Context, att *types.SignedAttestation) error {
	data, err := att.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	compressed := gossipcfg.CompressMessage(data)
	return s.attestationTopic.Publish(ctx, compressed)
}
