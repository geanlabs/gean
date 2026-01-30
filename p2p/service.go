package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// Service manages p2p networking for the consensus client.
type Service struct {
	host     host.Host
	pubsub   *pubsub.PubSub
	handlers *MessageHandlers
	logger   *slog.Logger

	blockTopic *pubsub.Topic
	blockSub   *pubsub.Subscription
	voteTopic  *pubsub.Topic
	voteSub    *pubsub.Subscription

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ServiceConfig holds configuration for the p2p service.
type ServiceConfig struct {
	Host      host.Host
	Handlers  *MessageHandlers
	Bootnodes []peer.AddrInfo
	Logger    *slog.Logger
}

// NewService creates a new p2p service.
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

	voteTopic, err := ps.Join(VoteTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join vote topic: %w", err)
	}

	// Subscribe to topics
	blockSub, err := blockTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe block topic: %w", err)
	}

	voteSub, err := voteTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe vote topic: %w", err)
	}

	svc := &Service{
		host:       cfg.Host,
		pubsub:     ps,
		handlers:   cfg.Handlers,
		logger:     logger,
		blockTopic: blockTopic,
		blockSub:   blockSub,
		voteTopic:  voteTopic,
		voteSub:    voteSub,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Connect to bootnodes
	for _, pi := range cfg.Bootnodes {
		if err := cfg.Host.Connect(ctx, pi); err != nil {
			logger.Warn("failed to connect to bootnode",
				"peer", pi.ID,
				"error", err,
			)
		} else {
			logger.Info("connected to bootnode", "peer", pi.ID)
		}
	}

	return svc, nil
}

// Start begins processing incoming messages.
func (s *Service) Start() {
	s.wg.Add(2)
	go s.processBlocks()
	go s.processVotes()
	s.logger.Info("p2p service started",
		"peer_id", s.host.ID(),
		"addrs", s.host.Addrs(),
	)
}

// Stop shuts down the p2p service.
func (s *Service) Stop() {
	s.cancel()
	s.blockSub.Cancel()
	s.voteSub.Cancel()
	s.wg.Wait()
	s.host.Close()
	s.logger.Info("p2p service stopped")
}

// PublishBlock publishes a signed block to the network.
func (s *Service) PublishBlock(ctx context.Context, block *types.SignedBlock) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	compressed := CompressMessage(data)
	return s.blockTopic.Publish(ctx, compressed)
}

// PublishVote publishes a signed vote to the network.
func (s *Service) PublishVote(ctx context.Context, vote *types.SignedVote) error {
	data, err := vote.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal vote: %w", err)
	}
	compressed := CompressMessage(data)
	return s.voteTopic.Publish(ctx, compressed)
}

// PeerCount returns the number of connected peers.
func (s *Service) PeerCount() int {
	return len(s.host.Network().Peers())
}

// Host returns the underlying libp2p host.
func (s *Service) Host() host.Host {
	return s.host
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

// processVotes handles incoming vote messages.
func (s *Service) processVotes() {
	defer s.wg.Done()

	for {
		msg, err := s.voteSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("vote subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleVoteMessage(s.ctx, msg.Data); err != nil {
				s.logger.Error("handle vote error", "error", err)
			}
		}
	}
}
