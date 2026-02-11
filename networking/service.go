// service.go contains networking service types and constructor wiring.
package networking

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gossipcfg "github.com/devylongs/gean/networking/gossipsub"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
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

const bootnodeRetryInterval = 30 * time.Second

// NewService creates a new networking service.
func NewService(ctx context.Context, cfg ServiceConfig) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create gossipsub
	ps, err := gossipcfg.New(ctx, cfg.Host)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	// Join topics
	blockTopic, err := ps.Join(gossipcfg.BlockTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join block topic: %w", err)
	}

	attestationTopic, err := ps.Join(gossipcfg.AttestationTopic)
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
