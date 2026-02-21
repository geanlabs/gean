package reqresp

import (
	"time"

	"github.com/geanlabs/gean/types"
)

// Protocol IDs matching cross-client convention (ssz_snappy encoding suffix).
const (
	StatusProtocol       = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
)

// Response status codes.
const (
	ResponseSuccess             = 0x00
	ResponseInvalidRequest      = 0x01
	ResponseServerError         = 0x02
	ResponseResourceUnavailable = 0x03
)

const reqRespTimeout = 10 * time.Second

// Status is the status message exchanged between peers.
type Status struct {
	Finalized *types.Checkpoint
	Head      *types.Checkpoint
}

// ReqRespHandler processes incoming request/response messages.
type ReqRespHandler struct {
	OnStatus       func(Status) Status
	OnBlocksByRoot func([][32]byte) []*types.SignedBlockWithAttestation
}
