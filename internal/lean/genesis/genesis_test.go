package genesis

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/lean/types"
)

const testConfigYAML = `GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "cd323f232b34ab26d6db7402c886e74ca81cfd3a0c659d2fe022356f25592f7d"
    proposal_pubkey: "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
  - attestation_pubkey: "b7b0f72e24801b02bda64073cb4de6699a416b37dfead227d7ca3922647c940f"
    proposal_pubkey: "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1"
  - attestation_pubkey: "8d9cbc508b20ef43e165f8559c1bdd18aaeda805ef565a4f9ffd6e4fbed01c05"
    proposal_pubkey: "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
`

func pubkeyHex(value byte) string {
	return strings.Repeat(fmt.Sprintf("%02x", value), types.PubkeySize)
}

func TestLoadGenesisConfig(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.GenesisTime != 1770407233 {
		t.Fatalf("genesis time: expected 1770407233, got %d", config.GenesisTime)
	}
	if len(config.GenesisValidators) != 3 {
		t.Fatalf("validators: expected 3, got %d", len(config.GenesisValidators))
	}
}

func TestValidators(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	validators, err := config.Validators()
	if err != nil {
		t.Fatalf("validators: %v", err)
	}

	if len(validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(validators))
	}
	for i, v := range validators {
		if v.Index != uint64(i) {
			t.Fatalf("validator %d index: expected %d, got %d", i, i, v.Index)
		}
		if v.AttestationPubkey == [types.PubkeySize]byte{} {
			t.Fatalf("validator %d has zero attestation pubkey", i)
		}
		if v.ProposalPubkey == [types.PubkeySize]byte{} {
			t.Fatalf("validator %d has zero proposal pubkey", i)
		}
		if v.AttestationPubkey == v.ProposalPubkey {
			t.Fatalf("validator %d: attestation and proposal pubkeys should be different", i)
		}
	}
}

func TestGenesisState(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}

	if state.Slot != 0 {
		t.Fatalf("genesis slot should be 0, got %d", state.Slot)
	}
	if state.Config.GenesisTime != 1770407233 {
		t.Fatal("genesis time mismatch")
	}
	if len(state.Validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(state.Validators))
	}
	if !types.IsZeroRoot(state.LatestJustified.Root) {
		t.Fatal("justified root should be zero at genesis")
	}
	if !types.IsZeroRoot(state.LatestFinalized.Root) {
		t.Fatal("finalized root should be zero at genesis")
	}
}

func TestLoadGenesisConfigMissing(t *testing.T) {
	_, err := LoadGenesisConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("should error on missing file")
	}
}

func TestLoadGenesisConfigValidatorCountConsistent(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, "VALIDATOR_COUNT: 3\n"+testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.ValidatorCount == nil {
		t.Fatal("ValidatorCount should be parsed when VALIDATOR_COUNT is set")
	}
	if *config.ValidatorCount != 3 {
		t.Fatalf("ValidatorCount: expected 3, got %d", *config.ValidatorCount)
	}
}

// TestLoadGenesisConfigAcceptsLeanchainFields covers the shared leanchain
// config.yaml schema other clients also consume (committee count, key lifetime,
// validator count) — these must parse, not trip strict field checking.
func TestLoadGenesisConfigAcceptsLeanchainFields(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile,
		"ATTESTATION_COMMITTEE_COUNT: 1\nACTIVE_EPOCH: 18\nVALIDATOR_COUNT: 3\n"+testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load leanchain config: %v", err)
	}
	if config.ActiveEpoch != 18 {
		t.Fatalf("ACTIVE_EPOCH not parsed: %d", config.ActiveEpoch)
	}
	if config.AttestationCommitteeCount == nil || *config.AttestationCommitteeCount != types.AttestationCommitteeCount {
		t.Fatalf("ATTESTATION_COMMITTEE_COUNT not parsed: %v", config.AttestationCommitteeCount)
	}
}

// Committee count is a shared network parameter; a multi-subnet devnet
// config.yaml must load rather than be rejected for differing from the default.
func TestLoadGenesisConfigAcceptsMultiSubnetCommitteeCount(t *testing.T) {
	for _, count := range []uint64{2, 8} {
		tmpFile := t.TempDir() + "/config.yaml"
		writeGenesisConfig(t, tmpFile, fmt.Sprintf("ATTESTATION_COMMITTEE_COUNT: %d\n", count)+testConfigYAML)

		config, err := LoadGenesisConfig(tmpFile)
		if err != nil {
			t.Fatalf("committee count %d: unexpected error: %v", count, err)
		}
		if config.AttestationCommitteeCount == nil || *config.AttestationCommitteeCount != count {
			t.Fatalf("committee count %d not parsed: %v", count, config.AttestationCommitteeCount)
		}
	}
}

func TestValidatorsReturnErrorsForDirectMalformedConfig(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "not-hex",
			ProposalPubkey:    "also-not-hex",
		}},
	}

	validators, err := config.Validators()
	if err == nil {
		t.Fatalf("expected validator error, got validators=%v", validators)
	}
}

func TestGenesisStateReturnsValidatorErrors(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "not-hex",
			ProposalPubkey:    "also-not-hex",
		}},
	}

	state, err := config.GenesisState()
	if err == nil {
		t.Fatalf("expected genesis state error, got state=%v", state)
	}
}

func TestGenesisConfigRejectsTooManyValidators(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime:       1,
		GenesisValidators: make([]GenesisValidatorEntry, types.ValidatorRegistryLimit+1),
	}

	validators, err := config.Validators()
	if err == nil {
		t.Fatalf("expected validator limit error, got validators=%v", validators)
	}
	if !strings.Contains(err.Error(), "max") {
		t.Fatalf("error %q does not mention max validators", err.Error())
	}
}

func TestValidatorsAcceptUppercaseHexPrefix(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "0X" + pubkeyHex(1),
			ProposalPubkey:    "0X" + pubkeyHex(2),
		}},
	}

	validators, err := config.Validators()
	if err != nil {
		t.Fatalf("validators: %v", err)
	}
	if len(validators) != 1 {
		t.Fatalf("validators=%d, want 1", len(validators))
	}
}

// GenesisState/Validators are pure constructors and must accept GenesisTime==0:
// leanSpec's API/test fixtures use genesisTime=0 for deterministic states. The
// time!=0 requirement is enforced on the config-file boundary only (see
// TestLoadGenesisConfigRejects).
func TestGenesisStateAcceptsZeroGenesisTime(t *testing.T) {
	config := &GenesisConfig{
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: pubkeyHex(1),
			ProposalPubkey:    pubkeyHex(2),
		}},
	}

	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("GenesisState with GenesisTime=0 and valid validators should succeed, got %v", err)
	}
	if state.Config.GenesisTime != 0 {
		t.Fatalf("expected GenesisTime=0 in state, got %d", state.Config.GenesisTime)
	}
}

// validatorsYAML builds a config with one validator per (attestation, proposal)
// pubkey pair.
func validatorsYAML(pubkeys ...string) string {
	data := "GENESIS_TIME: 1770407233\nGENESIS_VALIDATORS:\n"
	for i := 0; i < len(pubkeys); i += 2 {
		data += fmt.Sprintf("  - attestation_pubkey: %q\n    proposal_pubkey: %q\n", pubkeys[i], pubkeys[i+1])
	}
	return data
}

func TestLoadGenesisConfigRejects(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"invalid pubkey", validatorsYAML("not-hex", pubkeyHex(0xa1)), "invalid attestation pubkey"},
		{"zero pubkey", validatorsYAML(pubkeyHex(0), pubkeyHex(1)), "zero pubkey"},
		{"duplicate attestation pubkey",
			validatorsYAML(pubkeyHex(1), pubkeyHex(2), pubkeyHex(1), pubkeyHex(3)),
			"duplicate attestation pubkey"},
		{"same pubkey for both roles of one validator",
			validatorsYAML(pubkeyHex(1), pubkeyHex(1)),
			"duplicate proposal pubkey already used as attestation pubkey"},
		{"proposal pubkey reuses another validator's attestation pubkey",
			validatorsYAML(pubkeyHex(1), pubkeyHex(2), pubkeyHex(3), pubkeyHex(1)),
			"duplicate proposal pubkey already used as attestation pubkey at index 0"},
		{"validator count mismatch", "VALIDATOR_COUNT: 5\n" + testConfigYAML, "VALIDATOR_COUNT=5 disagrees"},
		{"zero committee count", "ATTESTATION_COMMITTEE_COUNT: 0\n" + testConfigYAML, "ATTESTATION_COMMITTEE_COUNT must be >= 1"},
		{"unknown field", "UNKNOWN_FIELD: true\n" + testConfigYAML, "UNKNOWN_FIELD"},
		{"multiple documents", testConfigYAML + "\n---\nGENESIS_TIME: 1\n", "single YAML document"},
		// The slot clock needs a real anchor, so a running node's config.yaml
		// with GENESIS_TIME=0 must be rejected at load time.
		{"zero genesis time", "GENESIS_TIME: 0\n" + strings.SplitN(testConfigYAML, "\n", 2)[1], "GENESIS_TIME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := t.TempDir() + "/config.yaml"
			writeGenesisConfig(t, tmpFile, tt.yaml)

			config, err := LoadGenesisConfig(tmpFile)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q (config=%v)", err, tt.wantErr, config)
			}
		})
	}
}

func writeGenesisConfig(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write genesis config: %v", err)
	}
}
