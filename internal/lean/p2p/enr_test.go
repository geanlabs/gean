package p2p

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
)

// A host's ENR advertises the attestation subnets it joined, in a field of
// committee-count bits as ethlambda writes it, and follows the aggregator role,
// including a switch made before discovery started.
func TestENRAdvertisesSubnetsAndAggregatorRole(t *testing.T) {
	// Validators 1 and 3 select subnets 1 and 3; aggregate subnet 5 has no
	// committee among 4 and cannot be advertised.
	h, err := NewHost(context.Background(), testHostConfig(t, nil), 4, []uint64{1, 3}, true, []uint64{5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	h.AdvertiseAggregator(true)
	if err := h.StartDiscovery(0, net.IPv4(127, 0, 0, 1), nil); err != nil {
		t.Fatal(err)
	}

	record := h.Node().Record()
	var attnets attnetsEntry
	if err := record.Load(&attnets); err != nil || !bytes.Equal(attnets, []byte{0b1010}) {
		t.Errorf("attnets: got %08b (%v), want the one byte 00001010", attnets, err)
	}
	var eth2 eth2Entry
	if err := record.Load(&eth2); err != nil || eth2 != localEth2 {
		t.Errorf("eth2: got %x (%v), want %x", eth2, err, localEth2)
	}
	var aggregator aggregatorEntry
	if err := record.Load(&aggregator); err != nil || !bytes.Equal(aggregator, isAggregatorEntry) {
		t.Errorf("is_aggregator: got %x (%v), want 01", aggregator, err)
	}

	h.AdvertiseAggregator(false)
	if err := h.Node().Record().Load(&aggregator); err == nil {
		t.Error("ENR still advertises the aggregator role after it was switched off")
	}
}

// Discovered nodes are dialed only when their eth2 entry carries gean's fork
// digest, whatever fork they plan next.
func TestSameFork(t *testing.T) {
	otherDigest, nextFork := localEth2, localEth2
	otherDigest[0] ^= 0xff
	nextFork[4], nextFork[8] = 0x01, 100
	for _, tc := range []struct {
		name  string
		entry enr.Entry
		want  bool
	}{
		{"gean's fork", localEth2, true},
		{"a different next fork", nextFork, true},
		{"another fork digest", otherDigest, false},
		{"no eth2 entry", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var record enr.Record
			if tc.entry != nil {
				record.Set(tc.entry)
			}
			if got := SameFork(enode.SignNull(&record, enode.ID{})); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
