package p2p

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
