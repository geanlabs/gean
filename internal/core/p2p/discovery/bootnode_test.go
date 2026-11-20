package discovery

import (
	"os"
	"strings"
	"testing"
)

func TestLoadBootnodesEmpty(t *testing.T) {
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte("# empty\n"), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("should not error on empty: %v", err)
	}
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addrs, got %d", len(addrs))
	}
}

func TestLoadBootnodesWithYAMLList(t *testing.T) {
	content := `- "/ip4/127.0.0.1/udp/9000/quic-v1/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"
- "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/12D3KooWLc4yBi3vYo4udihGu2HFxCWMWCdJoXYMFNp2CX9otY5A"
`
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte(content), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
}

func TestLoadBootnodesRejectsInvalidLine(t *testing.T) {
	tmpFile := t.TempDir() + "/nodes.yaml"
	if err := os.WriteFile(tmpFile, []byte("not-a-bootnode\n"), 0644); err != nil {
		t.Fatalf("write bootnodes: %v", err)
	}

	addrs, err := LoadBootnodes(tmpFile)
	if err == nil {
		t.Fatalf("expected invalid line error, got addrs=%v", addrs)
	}
	if !strings.Contains(err.Error(), "invalid bootnode line 1") {
		t.Fatalf("error=%v, want invalid line with number", err)
	}
}
