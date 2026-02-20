package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/config"
)

func TestLoadGenesisConfigParsesValidators(t *testing.T) {
	yaml := `
GENESIS_TIME: 1704085200
GENESIS_VALIDATORS:
  - "e2a03c16122c7e0f940e2301aa460c54a2e1e8343968bb2782f26636f051e65ec589c858b9c7980b276ebe550056b23f0bdc3b5a"
  - "0767e65924063f79ae92ee1953685f06718b1756cc665a299bd61b4b82055e377237595d9a27887421b5233d09a50832db2f303d"
  - "d4355005bc37f76f390dcd2bcc51677d8c6ab44e0cc64913fb84ad459789a31105bd9a69afd2690ffd737d22ec6e3b31d47a642f"
`
	path := writeTempYAML(t, yaml)
	cfg, err := config.LoadGenesisConfig(path)
	if err != nil {
		t.Fatalf("LoadGenesisConfig: %v", err)
	}

	if cfg.GenesisTime != 1704085200 {
		t.Fatalf("GenesisTime = %d, want 1704085200", cfg.GenesisTime)
	}
	if len(cfg.Validators) != 3 {
		t.Fatalf("len(Validators) = %d, want 3", len(cfg.Validators))
	}
	for i, v := range cfg.Validators {
		if v.Index != uint64(i) {
			t.Errorf("Validators[%d].Index = %d, want %d", i, v.Index, i)
		}
		if v.Pubkey == [52]byte{} {
			t.Errorf("Validators[%d].Pubkey is zero", i)
		}
	}

	// First byte of first pubkey should be 0xe2.
	if cfg.Validators[0].Pubkey[0] != 0xe2 {
		t.Errorf("Validators[0].Pubkey[0] = %x, want e2", cfg.Validators[0].Pubkey[0])
	}
}

func TestLoadGenesisConfigAccepts0xPrefix(t *testing.T) {
	yaml := `
GENESIS_TIME: 1000
GENESIS_VALIDATORS:
  - "0xe2a03c16122c7e0f940e2301aa460c54a2e1e8343968bb2782f26636f051e65ec589c858b9c7980b276ebe550056b23f0bdc3b5a"
`
	path := writeTempYAML(t, yaml)
	cfg, err := config.LoadGenesisConfig(path)
	if err != nil {
		t.Fatalf("LoadGenesisConfig: %v", err)
	}
	if len(cfg.Validators) != 1 {
		t.Fatalf("len(Validators) = %d, want 1", len(cfg.Validators))
	}
	if cfg.Validators[0].Pubkey[0] != 0xe2 {
		t.Errorf("Validators[0].Pubkey[0] = %x, want e2", cfg.Validators[0].Pubkey[0])
	}
}

func TestLoadGenesisConfigRejectsEmptyValidators(t *testing.T) {
	yaml := `
GENESIS_TIME: 1000
GENESIS_VALIDATORS: []
`
	path := writeTempYAML(t, yaml)
	_, err := config.LoadGenesisConfig(path)
	if err == nil {
		t.Fatal("expected error for empty validators")
	}
}

func TestLoadGenesisConfigRejectsWrongPubkeyLength(t *testing.T) {
	yaml := `
GENESIS_TIME: 1000
GENESIS_VALIDATORS:
  - "aabbcc"
`
	path := writeTempYAML(t, yaml)
	_, err := config.LoadGenesisConfig(path)
	if err == nil {
		t.Fatal("expected error for wrong pubkey length")
	}
}

func TestLoadGenesisConfigRejectsInvalidHex(t *testing.T) {
	yaml := `
GENESIS_TIME: 1000
GENESIS_VALIDATORS:
  - "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
`
	path := writeTempYAML(t, yaml)
	_, err := config.LoadGenesisConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
