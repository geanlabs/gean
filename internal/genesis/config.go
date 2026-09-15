package genesis

type GenesisValidatorEntry struct {
	AttestationPubkey string `yaml:"attestation_pubkey"`
	ProposalPubkey    string `yaml:"proposal_pubkey"`
}

type GenesisConfig struct {
	GenesisTime               uint64                  `yaml:"GENESIS_TIME"`
	AttestationCommitteeCount *uint64                 `yaml:"ATTESTATION_COMMITTEE_COUNT,omitempty"`
	ActiveEpoch               uint64                  `yaml:"ACTIVE_EPOCH,omitempty"`
	ValidatorCount            *uint64                 `yaml:"VALIDATOR_COUNT,omitempty"`
	GenesisValidators         []GenesisValidatorEntry `yaml:"GENESIS_VALIDATORS"`
	// ExecutionGenesisHash is the execution layer's block 0 hash. Its
	// presence declares that the network runs an execution layer: it is
	// committed into the genesis state, so every node must agree on it, and a
	// node pairs with an execution client only when it is set.
	ExecutionGenesisHash string `yaml:"EXECUTION_GENESIS_BLOCK_HASH,omitempty"`
}
