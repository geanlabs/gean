package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	libp2p_crypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// LocalNodeManager manages the local node's ENR and identity.
type LocalNodeManager struct {
	db      *enode.DB
	local   *enode.LocalNode
	privKey *ecdsa.PrivateKey
}

// NewLocalNodeManager creates a new local node manager.
// It loads the node key from the given path (or generates one) and opens the node DB.
func NewLocalNodeManager(dbPath string, nodeKeyPath string, ip net.IP, udpPort int, tcpPort int) (*LocalNodeManager, error) {
	// 1. Load or generate node key
	privKey, err := loadOrGenerateNodeKey(nodeKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node key: %w", err)
	}

	// 2. Initialize Node DB
	db, err := enode.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open node db: %w", err)
	}

	// 3. Create Local Node
	local := enode.NewLocalNode(db, privKey)

	// 4. Set ENR entries
	local.Set(enr.IP(ip))
	local.Set(enr.UDP(udpPort))
	// We might use TCP for libp2p later, or just for compat
	if tcpPort != 0 {
		local.Set(enr.TCP(tcpPort))
	}

	// Add a custom field to identify "gean" nodes?
	// local.Set(enr.WithEntry("client", "gean"))

	return &LocalNodeManager{
		db:      db,
		local:   local,
		privKey: privKey,
	}, nil
}

func (m *LocalNodeManager) Node() *enode.Node {
	return m.local.Node()
}

func (m *LocalNodeManager) Database() *enode.DB {
	return m.db
}

func (m *LocalNodeManager) PrivateKey() *ecdsa.PrivateKey {
	return m.privKey
}

func (m *LocalNodeManager) Close() {
	m.db.Close()
}

// ENRToAddrInfo parses an ENR string and returns a libp2p AddrInfo with a QUIC multiaddr.
func ENRToAddrInfo(enrStr string) (*peer.AddrInfo, error) {
	node, err := enode.Parse(enode.ValidSchemes, enrStr)
	if err != nil {
		return nil, fmt.Errorf("parse enr: %w", err)
	}

	ip := node.IP()
	if ip == nil {
		return nil, fmt.Errorf("enr has no IP")
	}

	var quicPort enr.QUIC
	if err := node.Record().Load(&quicPort); err != nil {
		return nil, fmt.Errorf("enr has no quic port: %w", err)
	}

	pubkey := node.Pubkey()
	if pubkey == nil {
		return nil, fmt.Errorf("enr has no public key")
	}
	compressed := crypto.CompressPubkey(pubkey)
	libp2pKey, err := libp2p_crypto.UnmarshalSecp256k1PublicKey(compressed)
	if err != nil {
		return nil, fmt.Errorf("convert pubkey: %w", err)
	}
	pid, err := peer.IDFromPublicKey(libp2pKey)
	if err != nil {
		return nil, fmt.Errorf("derive peer id: %w", err)
	}

	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", ip, quicPort))
	if err != nil {
		return nil, fmt.Errorf("build multiaddr: %w", err)
	}

	return &peer.AddrInfo{ID: pid, Addrs: []ma.Multiaddr{addr}}, nil
}

// loadOrGenerateNodeKey loads a secp256k1 key from file or generates a new one.
func loadOrGenerateNodeKey(path string) (*ecdsa.PrivateKey, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		if err := crypto.SaveECDSA(path, key); err != nil {
			return nil, err
		}
		return key, nil
	}
	key, err := crypto.LoadECDSA(path)
	if err == nil {
		return key, nil
	}

	// Try loading as raw binary (32 bytes) or Libp2p marshaled key
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	if len(data) == 32 {
		return crypto.ToECDSA(data)
	}

	// Try unmarshaling as Libp2p key
	sk, err := libp2p_crypto.UnmarshalPrivateKey(data)
	if err == nil {
		raw, err := sk.Raw()
		if err != nil {
			return nil, fmt.Errorf("failed to get raw key bytes: %w", err)
		}
		return crypto.ToECDSA(raw)
	}

	return nil, fmt.Errorf("invalid key format (hex, binary, or libp2p): %w", err)
}
