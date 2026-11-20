package host

import (
	"fmt"
	"net"
	"strconv"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/p2p/discovery"
)

// StartDiscovery runs discv5 on udpPort with the host's key, advertising its
// QUIC address (at advertiseIP, when set), and dials the peers it finds until
// Close.
func (h *Host) StartDiscovery(udpPort int, advertiseIP net.IP, bootnodes []*enode.Node) error {
	raw, err := h.host.Peerstore().PrivKey(h.host.ID()).Raw()
	if err != nil {
		return fmt.Errorf("discv5 key: %w", err)
	}
	key, err := gethcrypto.ToECDSA(raw)
	if err != nil {
		return fmt.Errorf("discv5 key: %w", err)
	}
	quicPort, err := h.quicPort()
	if err != nil {
		return err
	}

	svc, err := discovery.Start(discovery.Config{
		PrivateKey:  key,
		Port:        udpPort,
		AdvertiseIP: advertiseIP,
		QUICPort:    quicPort,
		Bootnodes:   bootnodes,
	})
	if err != nil {
		return err
	}
	h.discovery = svc
	h.tasks.Go(func() { svc.Dial(h.ctx, h.ConnectedPeers, h.ConnectPeer) })
	logger.Info(logger.Network, "discv5 started bootnodes=%d enr=%s", len(bootnodes), svc.Self())
	return nil
}

// Node returns this host's discv5 record, or nil when discovery is not running.
func (h *Host) Node() *enode.Node {
	if h.discovery == nil {
		return nil
	}
	return h.discovery.Self()
}

// quicPort returns the UDP port the host's QUIC listener is bound to.
func (h *Host) quicPort() (int, error) {
	for _, addr := range h.host.Network().ListenAddresses() {
		if _, err := addr.ValueForProtocol(multiaddr.P_QUIC_V1); err != nil {
			continue
		}
		port, err := addr.ValueForProtocol(multiaddr.P_UDP)
		if err != nil {
			continue
		}
		return strconv.Atoi(port)
	}
	return 0, fmt.Errorf("host has no QUIC listener")
}
