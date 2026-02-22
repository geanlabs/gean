package p2p

import (
	"fmt"
	"net"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"

	"github.com/geanlabs/gean/observability/logging"
)

// DiscoveryService manages peer discovery using Discv5.
type DiscoveryService struct {
	manager *LocalNodeManager
	udp     *discover.UDPv5
	port    int
}

// NewDiscoveryService starts a Discv5 service.
func NewDiscoveryService(manager *LocalNodeManager, port int, bootnodes []string) (*DiscoveryService, error) {
	log := logging.NewComponentLogger(logging.CompNetwork)

	// 1. Parse Bootnodes
	var boots []*enode.Node
	for _, url := range bootnodes {
		if url == "" {
			continue
		}
		node, err := enode.Parse(enode.ValidSchemes, url)
		if err != nil {
			log.Warn("invalid bootnode URL", "url", url, "err", err)
			continue
		}
		boots = append(boots, node)
	}

	// 2. Start UDP Listener
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	localAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve udp addr %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on udp %s: %w", addr, err)
	}

	cfg := discover.Config{
		PrivateKey: manager.PrivateKey(),
		Bootnodes:  boots,
	}

	// 3. Start Discovery
	udp, err := discover.ListenV5(conn, manager.local, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to start discv5: %w", err)
	}

	log.Info("discovery service started",
		"enr", manager.Node().String(),
		"id", manager.Node().ID().String(),
	)

	return &DiscoveryService{
		manager: manager,
		udp:     udp,
		port:    port,
	}, nil
}

func (s *DiscoveryService) Close() {
	s.udp.Close()
}

// LookupRandom finds random nodes in the DHT.
func (s *DiscoveryService) LookupRandom() []*enode.Node {
	iter := s.udp.RandomNodes()
	defer iter.Close()

	var nodes []*enode.Node
	for i := 0; i < 16 && iter.Next(); i++ {
		nodes = append(nodes, iter.Node())
	}
	return nodes
}

// Peers returns all nodes in the local table.
func (s *DiscoveryService) Peers() []*enode.Node {
	return s.udp.AllNodes()
}
