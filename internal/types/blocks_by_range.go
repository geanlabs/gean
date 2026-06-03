package types

type BlocksByRangeRequest struct {
	StartSlot uint64 `json:"start_slot"`
	Count     uint64 `json:"count"`
}
