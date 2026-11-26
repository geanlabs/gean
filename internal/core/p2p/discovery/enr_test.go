package discovery

import (
	"bytes"
	"encoding/base64"
	"net"
	"strings"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestParseENR(t *testing.T) {
	ip4, ip4ID := testNode(t, enr.IPv4{127, 0, 0, 1}, enr.QUIC(9001))
	both, bothID := testNode(t, enr.IPv4{10, 0, 0, 1}, enr.IPv6(net.ParseIP("2001:db8::1")), enr.QUIC(9001), enr.QUIC6(9002))
	tests := []struct {
		name, enr, want, wantErr string
	}{
		{"ip4 and quic", ip4.String(), "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/" + ip4ID.String(), ""},
		{"prefers ip4 over ip6", both.String(), "/ip4/10.0.0.1/udp/9001/quic-v1/p2p/" + bothID.String(), ""},
		{"altered signature", withFlippedSignature(t, ip4.String()), "", "invalid signature"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := ParseENR(tt.enr)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseENR: %v", err)
			}
			if got := ma.String(); got != tt.want {
				t.Fatalf("multiaddr = %q, want %q", got, tt.want)
			}
		})
	}
}

// withFlippedSignature returns enrStr with one bit of its signature flipped.
func withFlippedSignature(t *testing.T, enrStr string) string {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(enrStr, "enr:"))
	if err != nil {
		t.Fatal(err)
	}
	var elems []rlp.RawValue
	if err := rlp.DecodeBytes(data, &elems); err != nil {
		t.Fatal(err)
	}
	sig := elems[0]
	sig[len(sig)-1] ^= 1
	tampered, err := rlp.EncodeToBytes(elems)
	if err != nil {
		t.Fatal(err)
	}
	return "enr:" + base64.RawURLEncoding.EncodeToString(tampered)
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
			rec := &enrRecord{ip4: tc.ip4, ip6: tc.ip6, udpPort: tc.udpPort, quicPort: tc.quicPort, udp6Port: tc.udp6Port, quic6Port: tc.quic6Port}
			got := rec.transportMultiaddr()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// testENR is a record with an ip4 address and a quic port.
const testENR = "enr:-IW4QGGifTt9ypyMtChDISUNX3z4z5iPdiEPOmBoILvnDuWIKbWVmKXxZERPnw0piQyaBNCENFEPoIi-vxsnsrBig9MBgmlkgnY0gmlwhH8AAAGEcXVpY4IjKYlzZWNwMjU2azGhAhMMnGF1rmIPQ9tWgqfkNmvsG-aIyc9EJU5JFo3Tegys"

// FuzzParseENR feeds arbitrary records to the bootnode ENR parser, which must
// not panic on a malformed bootnode file. The fuzzer mutates the record's RLP,
// and the body signs what decodes, so mutations reach the entry parsing past
// the signature check.
func FuzzParseENR(f *testing.F) {
	record, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(testENR, "enr:"))
	if err != nil {
		f.Fatal(err)
	}
	key, err := gethcrypto.ToECDSA(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(record)
	f.Fuzz(func(t *testing.T, record []byte) {
		_, _ = ParseENR("enr:" + base64.RawURLEncoding.EncodeToString(record))
		var r enr.Record
		if rlp.DecodeBytes(record, &r) != nil || enode.SignV4(&r, key) != nil {
			return
		}
		signed, err := rlp.EncodeToBytes(&r)
		if err != nil {
			return
		}
		_, _ = ParseENR("enr:" + base64.RawURLEncoding.EncodeToString(signed))
	})
}
