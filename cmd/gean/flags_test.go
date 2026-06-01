package main

import (
	"bytes"
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

func TestParseConfig_InvalidCommitteeCount(t *testing.T) {
	args := append(validFlagArgs(), "--attestation-committee-count", "0")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected committee count validation to fail")
	}
	if !strings.Contains(stderr.String(), "--attestation-committee-count must be >= 1") {
		t.Fatalf("committee count message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_AggregateSubnetsRequireAggregator(t *testing.T) {
	args := append(validFlagArgs(), "--aggregate-subnet-ids", "1,2")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected aggregate subnets without aggregator to fail")
	}
	if !strings.Contains(stderr.String(), "--aggregate-subnet-ids requires --is-aggregator") {
		t.Fatalf("aggregate subnet dependency message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_InvalidAggregateSubnetID(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--aggregate-subnet-ids", "1,nope")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected invalid aggregate subnet ID to fail")
	}
	if !strings.Contains(stderr.String(), `invalid aggregate-subnet-id "nope"`) {
		t.Fatalf("invalid subnet message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_AggregateSubnetIDs(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--aggregate-subnet-ids", "1, 2,,3")
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
