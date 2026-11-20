package main

import (
	"bytes"
	"net"
	"reflect"
	"strings"
	"testing"
)

func validFlagArgs() []string {
	return []string{
		"--custom-network-config-dir", "/config",
		"--node-key", "/config/node0.key",
		"--node-id", "node0",
	}
}

func TestParseConfig_ValidDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}

	if cfg.ConfigDir != "/config" || cfg.NodeKey != "/config/node0.key" || cfg.NodeID != "node0" {
		t.Fatalf("required fields not parsed: %+v", cfg)
	}
	if cfg.GossipPort != 9000 || cfg.HTTPAddr != "127.0.0.1" || cfg.APIPort != 5052 || cfg.MetricsPort != 5054 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.IsAggregator || cfg.CommitteeCount != 1 || cfg.DataDir != "./data" || len(cfg.AggregateSubnetIDs) != 0 {
		t.Fatalf("unexpected role/storage defaults: %+v", cfg)
	}
	if cfg.ShadowAggregateSignaturesRate != 0 || cfg.ShadowVerifySignatureRate != 0 || cfg.ShadowVerifyAggregatedSignaturesRate != 0 {
		t.Fatalf("shadow rates must default to disabled: %+v", cfg)
	}
}

func TestParseConfig_ShadowRates(t *testing.T) {
	args := append(validFlagArgs(),
		"--shadow-xmss-aggregate-signatures-rate", "5",
		"--shadow-xmss-verify-signature-rate", "2",
		"--shadow-xmss-verify-aggregated-signatures-rate", "100",
	)
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 5 ||
		cfg.ShadowVerifySignatureRate != 2 ||
		cfg.ShadowVerifyAggregatedSignaturesRate != 100 {
		t.Fatalf("unexpected shadow rates: %+v", cfg)
	}
}

func TestParseConfig_ShadowRatesFromEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE", "7.5")
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE", "3")
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_AGGREGATED_SIGNATURES_RATE", "9")

	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 7.5 ||
		cfg.ShadowVerifySignatureRate != 3 ||
		cfg.ShadowVerifyAggregatedSignaturesRate != 9 {
		t.Fatalf("env rates not applied: %+v", cfg)
	}
}

func TestParseConfig_ShadowRateFlagBeatsEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE", "7.5")

	args := append(validFlagArgs(), "--shadow-xmss-aggregate-signatures-rate", "2")
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 2 {
		t.Fatalf("flag should win over env, got %v", cfg.ShadowAggregateSignaturesRate)
	}
}

func TestParseConfig_InvalidShadowEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE", "not-a-number")

	var stderr bytes.Buffer
	_, err := parseConfig(validFlagArgs(), &stderr)
	if err == nil {
		t.Fatal("expected unparseable env rate to fail")
	}
	if !strings.Contains(stderr.String(), "GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE") {
		t.Fatalf("env error message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_MissingRequiredFlags(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseConfig(nil, &stderr)
	if err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	if !strings.Contains(stderr.String(), "required flags: --custom-network-config-dir, --node-key, --node-id") {
		t.Fatalf("missing required flags message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_CommitteeCountSetTracking(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.CommitteeCountSet {
		t.Fatal("committeeCountSet should be false when flag omitted")
	}

	args := append(validFlagArgs(), "--attestation-committee-count", "4")
	cfg, err = parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.CommitteeCountSet || cfg.CommitteeCount != 4 {
		t.Fatalf("explicit flag not tracked: set=%v count=%d", cfg.CommitteeCountSet, cfg.CommitteeCount)
	}
}

func TestParseConfig_AggregateSubnetIDs(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--attestation-committee-count", "4", "--aggregate-subnet-ids", "1, 2,,3")
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if !cfg.IsAggregator {
		t.Fatal("expected aggregator flag to be true")
	}
	if !reflect.DeepEqual(cfg.AggregateSubnetIDs, []uint64{1, 2, 3}) {
		t.Fatalf("unexpected aggregate subnet IDs: %v", cfg.AggregateSubnetIDs)
	}
}

func TestParseConfig_Discovery(t *testing.T) {
	args := append(validFlagArgs(), "--discovery", "--discovery-port", "9100", "--discovery-advertise-ip", "127.0.0.1")
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if !cfg.Discovery || cfg.DiscoveryPort != 9100 || !cfg.DiscoveryAdvertiseIP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("discovery=%v port=%d ip=%v", cfg.Discovery, cfg.DiscoveryPort, cfg.DiscoveryAdvertiseIP)
	}
}

func TestParseConfig_DiscoveryOffByDefault(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.Discovery {
		t.Fatal("discovery should be off by default")
	}
}

func TestParseConfig_Rejects(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"negative aggregate signatures shadow rate", []string{"--shadow-xmss-aggregate-signatures-rate", "-1"}, "--shadow-xmss-aggregate-signatures-rate must not be negative"},
		{"negative verify signature shadow rate", []string{"--shadow-xmss-verify-signature-rate", "-1"}, "--shadow-xmss-verify-signature-rate must not be negative"},
		{"negative verify aggregated signatures shadow rate", []string{"--shadow-xmss-verify-aggregated-signatures-rate", "-1"}, "--shadow-xmss-verify-aggregated-signatures-rate must not be negative"},
		{"zero committee count", []string{"--attestation-committee-count", "0"}, "--attestation-committee-count must be >= 1"},
		{"aggregate subnets without aggregator", []string{"--aggregate-subnet-ids", "1,2"}, "--aggregate-subnet-ids requires --is-aggregator"},
		{"invalid aggregate subnet ID", []string{"--is-aggregator", "--aggregate-subnet-ids", "1,nope"}, `invalid aggregate-subnet-id "nope"`},
		{"aggregate subnet ID out of range", []string{"--is-aggregator", "--attestation-committee-count", "2", "--aggregate-subnet-ids", "0,2"}, "--aggregate-subnet-ids contains 2"},
		{"gossip port negative", []string{"--gossipsub-port", "-1"}, "--gossipsub-port must be between 0 and 65535"},
		{"api port too high", []string{"--api-port", "65536"}, "--api-port must be between 0 and 65535"},
		{"metrics port too high", []string{"--metrics-port", "65536"}, "--metrics-port must be between 0 and 65535"},
		{"discovery port too high", []string{"--discovery", "--discovery-port", "65536"}, "--discovery-port must be between 0 and 65535"},
		{"discovery port shared with gossipsub", []string{"--discovery", "--discovery-port", "9000"}, "--discovery-port must differ from --gossipsub-port"},
		{"advertise ip without discovery", []string{"--discovery-advertise-ip", "127.0.0.1"}, "--discovery-advertise-ip requires --discovery"},
		{"bad advertise ip", []string{"--discovery", "--discovery-advertise-ip", "not-an-ip"}, "is not an IP address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, err := parseConfig(append(validFlagArgs(), tt.args...), &stderr); err == nil {
				t.Fatal("expected parseConfig to fail")
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("message %q not found:\n%s", tt.want, stderr.String())
			}
		})
	}
}
