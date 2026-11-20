package discovery

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// testNode returns a signed record with the given entries, and the libp2p
// peer id of its key.
func testNode(t *testing.T, entries ...enr.Entry) (*enode.Node, peer.ID) {
	t.Helper()
	key, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	db, err := enode.OpenDB("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	local := enode.NewLocalNode(db, key)
	for _, e := range entries {
		local.Set(e)
	}
	pub, err := crypto.UnmarshalSecp256k1PublicKey(gethcrypto.CompressPubkey(&key.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return local.Node(), id
}

func TestQUICAddr(t *testing.T) {
	node, id := testNode(t, enr.IPv4(net.IPv4(10, 0, 0, 7)), enr.UDP(9001), enr.QUIC(9000))
	addr, err := QUICAddr(node)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/ip4/10.0.0.7/udp/9000/quic-v1/p2p/" + id.String(); addr.String() != want {
		t.Fatalf("addr=%s, want %s", addr, want)
	}
}

// The udp entry is discv5's own port, so a record without quic has no address
// to dial.
func TestQUICAddrRequiresQUICEntry(t *testing.T) {
	node, _ := testNode(t, enr.IPv4(net.IPv4(10, 0, 0, 7)), enr.UDP(9001))
	if _, err := QUICAddr(node); err == nil {
		t.Fatal("expected an error for a record without a quic entry")
	}
}

func TestLoadENRsSkipsMultiaddrs(t *testing.T) {
	node, _ := testNode(t, enr.IPv4(net.IPv4(10, 0, 0, 7)), enr.QUIC(9000))
	content := strings.Join([]string{
		"# bootnodes",
		"- " + node.String(),
		"- /ip4/127.0.0.1/udp/9000/quic-v1/p2p/16Uiu2HAmPQhkD6Zg5Co2ee8ShshkiY4tDePKFARPpCS2oKSLj1E1",
	}, "\n")
	path := filepath.Join(t.TempDir(), "nodes.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	nodes, err := LoadENRs(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID() != node.ID() {
		t.Fatalf("nodes=%v, want just %s", nodes, node.ID())
	}
}
