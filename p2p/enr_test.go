package p2p

import (
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
