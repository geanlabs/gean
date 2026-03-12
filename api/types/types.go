package types

// HealthResponse is the JSON response for the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// CheckpointResponse is the JSON response for checkpoint endpoints.
type CheckpointResponse struct {
	Slot uint64 `json:"slot"`
	Root string `json:"root"`
}

// ForkChoiceNode is a JSON-safe fork choice node response.
type ForkChoiceNode struct {
	Root          string `json:"root"`
	Slot          uint64 `json:"slot"`
	ParentRoot    string `json:"parent_root"`
	ProposerIndex uint64 `json:"proposer_index"`
	Weight        int    `json:"weight"`
}

// ForkChoiceResponse is the JSON response for the fork choice endpoint.
type ForkChoiceResponse struct {
	Nodes          []ForkChoiceNode   `json:"nodes"`
	Head           string             `json:"head"`
	Justified      CheckpointResponse `json:"justified"`
	Finalized      CheckpointResponse `json:"finalized"`
	SafeTarget     string             `json:"safe_target"`
	ValidatorCount uint64             `json:"validator_count"`
}
