package p2p

const (
	StatusProtocol        = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol  = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
	BlocksByRangeProtocol = "/leanconsensus/req/blocks_by_range/1/ssz_snappy"
)

// MaxSignedBlockSize is the SSZ size of a SignedBlock with every list and
// bitlist at its limit. A block response chunk declaring more cannot decode.
const MaxSignedBlockSize = 3_182_692
