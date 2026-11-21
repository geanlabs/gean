package discovery

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/internal/core/logger"
)

const (
	// TargetPeers is the connected-peer count above which discovered peers
	// are no longer dialed.
	TargetPeers = 50
	// redialAfter is how long a discovered peer is left alone after a dial,
	// since a lookup in a small network returns the same nodes again and again.
	redialAfter = 30 * time.Second
	// fullPause is how long the dial loop waits before checking again once the
	// node holds TargetPeers.
	fullPause = 5 * time.Second
)

// Config is what discv5 needs.
type Config struct {
	PrivateKey *ecdsa.PrivateKey
	// Port is the UDP port discv5 listens on. Zero picks a free one.
	Port int
	// AdvertiseIP is the IP put in this node's ENR. Nil leaves it to discv5,
	// which learns it from what its peers see.
	AdvertiseIP net.IP
	// QUICPort is the libp2p QUIC port advertised in the ENR's quic entry.
	QUICPort  int
	Bootnodes []*enode.Node
	// Entries are the chain's own ENR entries, such as its subnets.
	Entries []enr.Entry
}

// Service runs discv5 on its own UDP socket.
type Service struct {
	db    *enode.DB
	local *enode.LocalNode
	udp   *discover.UDPv5
}

// Start opens the discv5 socket and joins the network through cfg.Bootnodes.
func Start(cfg Config) (*Service, error) {
	db, err := enode.OpenDB("")
	if err != nil {
		return nil, fmt.Errorf("open node db: %w", err)
	}
	local := enode.NewLocalNode(db, cfg.PrivateKey)
	local.Set(enr.QUIC(cfg.QUICPort))
	for _, entry := range cfg.Entries {
		local.Set(entry)
	}
	if cfg.AdvertiseIP != nil {
		local.SetStaticIP(cfg.AdvertiseIP)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: cfg.Port})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("listen discv5 on udp port %d: %w", cfg.Port, err)
	}
	local.SetFallbackUDP(conn.LocalAddr().(*net.UDPAddr).Port)

	udp, err := discover.ListenV5(conn, local, discover.Config{
		PrivateKey: cfg.PrivateKey,
		Bootnodes:  cfg.Bootnodes,
	})
	if err != nil {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("start discv5: %w", err)
	}
	return &Service{db: db, local: local, udp: udp}, nil
}

// Set puts entry in this node's record, replacing any entry under its key.
// The record's sequence number rises, so peers fetch the new record.
func (s *Service) Set(entry enr.Entry) {
	s.local.Set(entry)
}

// Delete is the inverse of Set.
func (s *Service) Delete(entry enr.Entry) {
	s.local.Delete(entry)
}

// Self returns this node's current record.
func (s *Service) Self() *enode.Node {
	return s.udp.Self()
}

// Close stops discv5 and closes its socket.
func (s *Service) Close() {
	s.udp.Close()
	s.db.Close()
}

// Dial walks the network and dials each peer it finds with a QUIC address and,
// when admit is set, that admit accepts, while connected reports fewer than
// TargetPeers, until ctx ends or the service closes.
func (s *Service) Dial(ctx context.Context, admit func(*enode.Node) bool, connected func() int, dial func(context.Context, multiaddr.Multiaddr) error) {
	it := s.udp.RandomNodes()
	if admit != nil {
		it = enode.Filter(it, admit)
	}
	defer it.Close()
	stop := context.AfterFunc(ctx, it.Close)
	defer stop()

	dialed := make(map[enode.ID]time.Time)
	for it.Next() {
		for connected() >= TargetPeers {
			select {
			case <-ctx.Done():
				return
			case <-time.After(fullPause):
			}
		}
		node := it.Node()
		if last, ok := dialed[node.ID()]; ok && time.Since(last) < redialAfter {
			continue
		}
		addr, err := QUICAddr(node)
		if err != nil {
			continue
		}
		dialed[node.ID()] = time.Now()
		if err := dial(ctx, addr); err != nil {
			logger.Info(logger.Network, "dial discovered peer %s failed: %v", addr, err)
			continue
		}
		logger.Info(logger.Network, "connected to discovered peer %s", addr)
	}
}

// QUICAddr returns the libp2p QUIC address a node's record advertises: its ip
// and quic entries, or ip6 and quic6, with the peer id of its secp256k1 key.
func QUICAddr(node *enode.Node) (multiaddr.Multiaddr, error) {
	var ip4 enr.IPv4
	var quic enr.QUIC
	var ip6 enr.IPv6
	var quic6 enr.QUIC6
	var transport string
	switch {
	case node.Load(&ip4) == nil && node.Load(&quic) == nil:
		transport = fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", net.IP(ip4), quic)
	case node.Load(&ip6) == nil && node.Load(&quic6) == nil:
		transport = fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", net.IP(ip6), quic6)
	default:
		return nil, fmt.Errorf("record of %s has no ip and quic entries", node.ID())
	}

	pub := node.Pubkey()
	if pub == nil {
		return nil, fmt.Errorf("record of %s has no secp256k1 key", node.ID())
	}
	id, err := peerIDFromPubkey(pub)
	if err != nil {
		return nil, fmt.Errorf("peer id of %s: %w", node.ID(), err)
	}
	return multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", transport, id))
}
