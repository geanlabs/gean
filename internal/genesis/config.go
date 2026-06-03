package genesis

type GenesisValidatorEntry struct {
	AttestationPubkey string `yaml:"attestation_pubkey"`
	ProposalPubkey    string `yaml:"proposal_pubkey"`
}

type GenesisConfig struct {
	GenesisTime               uint64                  `yaml:"GENESIS_TIME"`
	AttestationCommitteeCount *uint64                 `yaml:"ATTESTATION_COMMITTEE_COUNT,omitempty"`
	ActiveEpoch               *uint64                 `yaml:"ACTIVE_EPOCH,omitempty"`
	ValidatorCount            *uint64                 `yaml:"VALIDATOR_COUNT,omitempty"`
	GenesisValidators         []GenesisValidatorEntry `yaml:"GENESIS_VALIDATORS"`
}
