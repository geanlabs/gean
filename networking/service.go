package networking

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type Service struct {
	host     host.Host
	pubsub   *pubsub.PubSub
	handlers *MessageHandlers
	logger   *slog.Logger

	blockTopic       *pubsub.Topic
	blockSub         *pubsub.Subscription
	attestationTopic *pubsub.Topic
	attestationSub   *pubsub.Subscription

	// Bootnodes that failed initial connection, to be retried.
	failedBootnodes []peer.AddrInfo

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ServiceConfig holds configuration for the networking service.
type ServiceConfig struct {
	Host      host.Host
	Handlers  *MessageHandlers
	Bootnodes []peer.AddrInfo
	Logger    *slog.Logger
}

// NewService creates a new networking service.
func NewService(ctx context.Context, cfg ServiceConfig) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create gossipsub
	ps, err := NewGossipSub(ctx, cfg.Host)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	// Join topics
	blockTopic, err := ps.Join(BlockTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join block topic: %w", err)
	}

	attestationTopic, err := ps.Join(AttestationTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join attestation topic: %w", err)
	}

	// Subscribe to topics
	blockSub, err := blockTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe block topic: %w", err)
	}

	attestationSub, err := attestationTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe attestation topic: %w", err)
	}

	svc := &Service{
		host:             cfg.Host,
		pubsub:           ps,
		handlers:         cfg.Handlers,
		logger:           logger,
		blockTopic:       blockTopic,
		blockSub:         blockSub,
		attestationTopic: attestationTopic,
		attestationSub:   attestationSub,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Connect to bootnodes, track failures for retry
	for _, pi := range cfg.Bootnodes {
		if err := cfg.Host.Connect(ctx, pi); err != nil {
			logger.Warn("failed to connect to bootnode",
				"peer", pi.ID,
				"error", err,
			)
			svc.failedBootnodes = append(svc.failedBootnodes, pi)
		} else {
			logger.Info("connected to bootnode", "peer", pi.ID)
		}
	}

	return svc, nil
}

func (s *Service) Start() {
	s.wg.Add(2)
	go s.processBlocks()
	go s.processAttestations()

	if len(s.failedBootnodes) > 0 {
		s.wg.Add(1)
		go s.retryBootnodes()
	}

	s.logger.Info("networking service started",
		"peer_id", s.host.ID(),
		"addrs", s.host.Addrs(),
	)
}

// Stop shuts down the networking service.
func (s *Service) Stop() {
	s.cancel()
	s.blockSub.Cancel()
	s.attestationSub.Cancel()
	s.wg.Wait()
	s.host.Close()
	s.logger.Info("networking service stopped")
}

// PublishBlock publishes a signed block to the network.
func (s *Service) PublishBlock(ctx context.Context, block *types.SignedBlockWithAttestation) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	compressed := CompressMessage(data)
	return s.blockTopic.Publish(ctx, compressed)
}

// PublishAttestation publishes a signed attestation to the network.
func (s *Service) PublishAttestation(ctx context.Context, att *types.SignedAttestation) error {
	data, err := att.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	compressed := CompressMessage(data)
	return s.attestationTopic.Publish(ctx, compressed)
}

// PeerCount returns the number of connected peers.
func (s *Service) PeerCount() int {
	return len(s.host.Network().Peers())
}

const bootnodeRetryInterval = 30 * time.Second

// retryBootnodes periodically retries connecting to failed bootnodes.
func (s *Service) retryBootnodes() {
	defer s.wg.Done()

	ticker := time.NewTicker(bootnodeRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			var remaining []peer.AddrInfo
			for _, pi := range s.failedBootnodes {
				if err := s.host.Connect(s.ctx, pi); err != nil {
					s.logger.Debug("bootnode reconnect failed", "peer", pi.ID, "error", err)
					remaining = append(remaining, pi)
				} else {
					s.logger.Info("reconnected to bootnode", "peer", pi.ID)
				}
			}
			s.failedBootnodes = remaining
			if len(s.failedBootnodes) == 0 {
				s.logger.Debug("all bootnodes connected, stopping retry")
				return
			}
		}
	}
}

// processBlocks handles incoming block messages.
func (s *Service) processBlocks() {
	defer s.wg.Done()

	for {
		msg, err := s.blockSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("block subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleBlockMessage(s.ctx, msg.Data, msg.ReceivedFrom); err != nil {
				s.logger.Error("handle block error", "error", err)
			}
		}
	}
}

// processAttestations handles incoming attestation messages.
func (s *Service) processAttestations() {
	defer s.wg.Done()

	for {
		msg, err := s.attestationSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("attestation subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleAttestationMessage(s.ctx, msg.Data); err != nil {
				s.logger.Error("handle attestation error", "error", err)
			}
		}
	}
}
