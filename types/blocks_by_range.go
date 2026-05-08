package types

// BlocksByRangeRequest is the SSZ payload of a /leanconsensus/req/blocks_by_range/1
// request. The server is asked to return up to Count blocks starting at StartSlot.
//
// Spec: leanSpec/src/lean_spec/subspecs/networking/reqresp/message.py BlocksByRangeRequest.
type BlocksByRangeRequest struct {
	StartSlot uint64 `json:"start_slot"`
	Count     uint64 `json:"count"`
}
