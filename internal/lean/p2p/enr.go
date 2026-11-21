package p2p

import (
	"encoding/binary"
	"encoding/hex"
	"math"

	"github.com/ethereum/go-ethereum/p2p/enode"
)

// attnetsEntry is the ENR attnets bitfield, bit i for subnet i. leanSpec
// specifies a 64-bit Bitvector; gean sizes it by the committee count, as
// ethlambda does, so the two read each other's records.
type attnetsEntry []byte

func (attnetsEntry) ENRKey() string { return "attnets" }

// encodeAttnets drops subnets at or above committeeCount, which have no
// committee.
func encodeAttnets(ids map[uint64]bool, committeeCount uint64) attnetsEntry {
	bits := make(attnetsEntry, (committeeCount+7)/8)
	for id := range ids {
		if id < committeeCount {
			bits[id/8] |= 1 << (id % 8)
		}
	}
	return bits
}

// aggregatorEntry is the ENR is_aggregator entry. leanSpec reads a node as an
// aggregator only when the entry is the single byte 0x01, so gean drops the
// entry rather than writing 0x00. ethlambda ignores it.
type aggregatorEntry []byte

func (aggregatorEntry) ENRKey() string { return "is_aggregator" }

var isAggregatorEntry = aggregatorEntry{0x01}

// AdvertiseAggregator follows the aggregator role in this node's ENR.
func (h *Host) AdvertiseAggregator(isAggregator bool) {
	if isAggregator {
		h.SetENREntry(isAggregatorEntry)
	} else {
		h.DeleteENREntry(isAggregatorEntry)
	}
}

// eth2Entry is the ENR eth2 entry, the beacon chain's SSZ ENRForkID: fork
// digest, next fork version, next fork epoch. Lean has no fork schedule, so
// gean writes what ethlambda does: its topic fork digest, then a zero version
// at the far-future epoch.
type eth2Entry [16]byte

func (eth2Entry) ENRKey() string { return "eth2" }

// localEth2 takes its digest from ForkDigest, so the ENR and the topic names
// cannot disagree.
var localEth2 = func() eth2Entry {
	var e eth2Entry
	if _, err := hex.Decode(e[:4], []byte(ForkDigest)); err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(e[8:], math.MaxUint64)
	return e
}()

// SameFork admits a discovered node whose eth2 entry carries gean's fork
// digest. As in ethlambda, a node without the entry is refused and its next
// fork is ignored.
func SameFork(node *enode.Node) bool {
	var e eth2Entry
	return node.Load(&e) == nil && [4]byte(e[:4]) == [4]byte(localEth2[:4])
}
