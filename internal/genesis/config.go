package genesis

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/geanlabs/gean/internal/types"
	"gopkg.in/yaml.v3"
)

// GenesisValidatorEntry pairs attestation and proposal pubkeys per validator.
type GenesisValidatorEntry struct {
	AttestationPubkey string `yaml:"attestation_pubkey"`
	ProposalPubkey    string `yaml:"proposal_pubkey"`
}

// GenesisConfig is parsed from config.yaml.
type GenesisConfig struct {
	GenesisTime uint64 `yaml:"GENESIS_TIME"`
	// NumValidators mirrors the spec's optional NUM_VALIDATORS field
	// (Uint64 | None). When present, it must equal len(GenesisValidators);
	// LoadGenesisConfig rejects the config otherwise. Absent in standard
	// lean-quickstart configs, so cross-client configs that include it
	// fail loudly rather than silently disagreeing on validator count.
	NumValidators     *uint64                 `yaml:"NUM_VALIDATORS,omitempty"`
	GenesisValidators []GenesisValidatorEntry `yaml:"GENESIS_VALIDATORS"`
}

// Validators converts genesis entries to typed Validators with sequential indices.
func (gc *GenesisConfig) Validators() []*types.Validator {
	validators := make([]*types.Validator, len(gc.GenesisValidators))
	for i, entry := range gc.GenesisValidators {
		attPk := parseHexPubkey(entry.AttestationPubkey, i, "attestation")
		propPk := parseHexPubkey(entry.ProposalPubkey, i, "proposal")
		validators[i] = &types.Validator{
			AttestationPubkey: attPk,
			ProposalPubkey:    propPk,
			Index:             uint64(i),
		}
	}
	return validators
}

func parseHexPubkey(hexStr string, index int, keyType string) [types.PubkeySize]byte {
	hexStr = strings.TrimPrefix(strings.TrimSpace(hexStr), "0x")
	pkBytes, err := hex.DecodeString(hexStr)
	if err != nil || len(pkBytes) != types.PubkeySize {
		panic(fmt.Sprintf("GENESIS_VALIDATORS[%d] invalid %s pubkey: %s", index, keyType, hexStr))
	}
	var pubkey [types.PubkeySize]byte
	copy(pubkey[:], pkBytes)
	return pubkey
}

// GenesisState creates the genesis state from config.
func (gc *GenesisConfig) GenesisState() *types.State {
	validators := gc.Validators()

	// Genesis block header with empty body root.
	emptyBody := &types.BlockBody{}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	return &types.State{
		Config: &types.ChainConfig{GenesisTime: gc.GenesisTime},
		Slot:   0,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    types.ZeroRoot,
			StateRoot:     types.ZeroRoot,
			BodyRoot:      bodyRoot,
		},
		LatestJustified:          &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

// LoadGenesisConfig reads and parses config.yaml.
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}
	var config GenesisConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	if config.GenesisTime == 0 {
		return nil, fmt.Errorf("GENESIS_TIME is 0 or missing")
	}
	if len(config.GenesisValidators) == 0 {
		return nil, fmt.Errorf("GENESIS_VALIDATORS is empty")
	}
	if config.NumValidators != nil && *config.NumValidators != uint64(len(config.GenesisValidators)) {
		return nil, fmt.Errorf("NUM_VALIDATORS=%d disagrees with len(GENESIS_VALIDATORS)=%d",
			*config.NumValidators, len(config.GenesisValidators))
	}
	return &config, nil
}
