package types

// ChainConfig holds minimal consensus configuration embedded in the beacon state.
type ChainConfig struct {
	GenesisTime uint64 `json:"genesis_time"`
}
