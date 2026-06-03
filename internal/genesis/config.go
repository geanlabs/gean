package genesis

type GenesisValidatorEntry struct {
	AttestationPubkey string `yaml:"attestation_pubkey"`
	ProposalPubkey    string `yaml:"proposal_pubkey"`
}

type GenesisConfig struct {
	GenesisTime       uint64                  `yaml:"GENESIS_TIME"`
	NumValidators     *uint64                 `yaml:"NUM_VALIDATORS,omitempty"`
	GenesisValidators []GenesisValidatorEntry `yaml:"GENESIS_VALIDATORS"`
}
