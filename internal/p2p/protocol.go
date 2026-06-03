package p2p

import "time"

const (
	StatusProtocol        = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol  = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
	BlocksByRangeProtocol = "/leanconsensus/req/blocks_by_range/1/ssz_snappy"
)

const ReqRespTimeout = 15 * time.Second
