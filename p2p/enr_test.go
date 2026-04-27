package p2p

import (
	"net"
	"strings"
	"testing"
)

func TestParseENR(t *testing.T) {
	// From lean-quickstart nodes.yaml
	enr := "enr:-IW4QGGifTt9ypyMtChDISUNX3z4z5iPdiEPOmBoILvnDuWIKbWVmKXxZERPnw0piQyaBNCENFEPoIi-vxsnsrBig9MBgmlkgnY0gmlwhH8AAAGEcXVpY4IjKYlzZWNwMjU2azGhAhMMnGF1rmIPQ9tWgqfkNmvsG-aIyc9EJU5JFo3Tegys"

	ma, err := ParseENR(enr)
	if err != nil {
		t.Fatalf("ParseENR failed: %v", err)
	}

	t.Logf("Parsed multiaddr: %s", ma.String())

	// Should contain ip4, udp, quic-v1, p2p components
	maStr := ma.String()
	if maStr == "" {
		t.Fatal("empty multiaddr")
	}
}

func TestBuildTransportMultiaddr(t *testing.T) {
	ip4 := net.IPv4(10, 0, 0, 1).To4()
	ip6 := net.ParseIP("2001:db8::1")

	tests := []struct {
		name                                   string
		ip4, ip6                               net.IP
		udpPort, quicPort, udp6Port, quic6Port uint16
		want                                   string
	}{
		{"ip4 quic only", ip4, nil, 0, 9001, 0, 0, "/ip4/10.0.0.1/udp/9001/quic-v1"},
		{"ip4 udp fallback", ip4, nil, 9001, 0, 0, 0, "/ip4/10.0.0.1/udp/9001/quic-v1"},
		{"ip6 quic6 only", nil, ip6, 0, 0, 0, 9002, "/ip6/2001:db8::1/udp/9002/quic-v1"},
		{"ip6 udp6 fallback", nil, ip6, 0, 0, 9002, 0, "/ip6/2001:db8::1/udp/9002/quic-v1"},
		{"both present prefers ip4", ip4, ip6, 0, 9001, 0, 9002, "/ip4/10.0.0.1/udp/9001/quic-v1"},
		{"ip6 reuses ip4 port when ip6 ports absent", nil, ip6, 0, 9001, 0, 0, "/ip6/2001:db8::1/udp/9001/quic-v1"},
		{"no ports returns empty", ip4, nil, 0, 0, 0, 0, ""},
		{"no ip returns empty", nil, nil, 9001, 9001, 0, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTransportMultiaddr(tc.ip4, tc.ip6, tc.udpPort, tc.quicPort, tc.udp6Port, tc.quic6Port)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseENR_PrefersIPv4 confirms that an ENR with both ip and ip6 yields an
// IPv4 multiaddr (matches leanSpec's test_enr_with_extensions fixture behavior).
func TestParseENR_PrefersIPv4(t *testing.T) {
	// ENR containing both ip4 and ip6 fields (from leanSpec test fixture).
	enr := "enr:-PK4QAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAFh2F0dG5ldHOIgQAAAAAAAICEZXRoMpASNFZ4q83vAGQAAAAAAAAAgmlkgnY0gmlwhAoAAAGDaXA2kCABDbgAAAAAAAAAAAAAAAGNaXNfYWdncmVnYXRvcgGEcXVpY4IjKYVxdWljNoIjKolzZWNwMjU2azGhA8pjTK4NSay0Adikxrb-jFW3DRFb9AB2nMFADzJYzTE4iHN5bmNuZXRzCoN1ZHCCdl-EdWRwNoJ2YA"
	ma, err := ParseENR(enr)
	if err != nil {
		t.Fatalf("ParseENR failed: %v", err)
	}
	maStr := ma.String()
	if !strings.HasPrefix(maStr, "/ip4/10.0.0.1/") {
		t.Errorf("expected /ip4/ preference, got %q", maStr)
	}
}
