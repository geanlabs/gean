package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

var errInvalidConfig = errors.New("invalid gean configuration")

type config struct {
	ConfigDir          string
	GossipPort         int
	HTTPAddr           string
	APIPort            int
	MetricsPort        int
	NodeKey            string
	NodeID             string
	CheckpointURL      string
	IsAggregator       bool
	ProverArena        bool
	CommitteeCount     uint64
	committeeCountSet  bool
	AggregateSubnetIDs []uint64
	DataDir            string

	// Execution-layer pairing. Endpoint and secret go together; both are
	// required on a network whose config declares an execution layer and
	// rejected on one that does not. FeeRecipient is the address the
	// execution client is asked to pay block rewards to.
	ExecutionEndpoint  string
	ExecutionJWTSecret string
	FeeRecipient       [types.AddressSize]byte

	// Shadow*Rate model XMSS prover cost for the Shadow network simulator, which
	// does not charge CPU time. Each is in signature-units per second; a
	// non-positive rate (the default) disables the delay, so real deployments are
	// unaffected.
	ShadowAggregateSignaturesRate        float64
	ShadowVerifySignatureRate            float64
	ShadowVerifyAggregatedSignaturesRate float64
}

type configPaths struct {
	config     string
	bootnodes  string
	validators string
	keysDir    string
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("gean", flag.ContinueOnError)
	fs.SetOutput(stderr)

	aggregateSubnetIDs := ""
	feeRecipient := ""
	fs.StringVar(&cfg.ConfigDir, "custom-network-config-dir", "", "Config directory (required)")
	fs.IntVar(&cfg.GossipPort, "gossipsub-port", 9000, "P2P listen port (QUIC/UDP)")
	fs.StringVar(&cfg.HTTPAddr, "http-address", "127.0.0.1", "Bind address for API + metrics")
	fs.IntVar(&cfg.APIPort, "api-port", 5052, "API server port")
	fs.IntVar(&cfg.MetricsPort, "metrics-port", 5054, "Metrics server port")
	fs.StringVar(&cfg.NodeKey, "node-key", "", "Path to hex-encoded secp256k1 private key (required)")
	fs.StringVar(&cfg.NodeID, "node-id", "", "Node identifier, e.g. gean_0 (required)")
	fs.StringVar(&cfg.CheckpointURL, "checkpoint-sync-url", "", "URL for checkpoint sync (optional)")
	fs.BoolVar(&cfg.IsAggregator, "is-aggregator", false, "Enable attestation aggregation")
	fs.BoolVar(&cfg.ProverArena, "prover-arena", false, "Prove on leanVM's bump arena instead of the system allocator: faster proving, but RSS ratchets to the high-water mark and never returns (off by default keeps memory bounded for packing many nodes per host)")
	fs.Uint64Var(&cfg.CommitteeCount, "attestation-committee-count", uint64(types.AttestationCommitteeCount), "Number of attestation subnets (overrides config.yaml ATTESTATION_COMMITTEE_COUNT)")
	fs.StringVar(&aggregateSubnetIDs, "aggregate-subnet-ids", "", "Comma-separated subnet IDs (requires --is-aggregator)")
	fs.StringVar(&cfg.DataDir, "data-dir", "./data", "Pebble database directory")
	fs.StringVar(&cfg.ExecutionEndpoint, "execution-endpoint", "", "Execution client Engine API endpoint, e.g. http://127.0.0.1:8551 (requires --execution-jwt-secret)")
	fs.StringVar(&cfg.ExecutionJWTSecret, "execution-jwt-secret", "", "Path to the hex JWT secret shared with the execution client")
	fs.StringVar(&feeRecipient, "suggested-fee-recipient", "", "20-byte hex address the execution client pays block rewards to")
	fs.Float64Var(&cfg.ShadowAggregateSignaturesRate, "shadow-xmss-aggregate-signatures-rate", 0, "Shadow simulator: signatures/sec rate for aggregation cost; n-signature op sleeps n/rate sec (0 disables; env GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE)")
	fs.Float64Var(&cfg.ShadowVerifySignatureRate, "shadow-xmss-verify-signature-rate", 0, "Shadow simulator: signatures/sec rate for gossip-attestation verify cost (0 disables; env GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE)")
	fs.Float64Var(&cfg.ShadowVerifyAggregatedSignaturesRate, "shadow-xmss-verify-aggregated-signatures-rate", 0, "Shadow simulator: signatures/sec rate for aggregated-signature verify cost (0 disables; env GEAN_SHADOW_XMSS_VERIFY_AGGREGATED_SIGNATURES_RATE)")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "attestation-committee-count" {
			cfg.committeeCountSet = true
		}
	})

	if cfg.ConfigDir == "" || cfg.NodeKey == "" || cfg.NodeID == "" {
		fmt.Fprintln(stderr, "required flags: --custom-network-config-dir, --node-key, --node-id")
		fs.Usage()
		return cfg, errInvalidConfig
	}
	if err := validatePort("gossipsub-port", cfg.GossipPort, stderr); err != nil {
		return cfg, err
	}
	if err := validatePort("api-port", cfg.APIPort, stderr); err != nil {
		return cfg, err
	}
	if err := validatePort("metrics-port", cfg.MetricsPort, stderr); err != nil {
		return cfg, err
	}
	if cfg.CommitteeCount < 1 {
		fmt.Fprintln(stderr, "--attestation-committee-count must be >= 1")
		return cfg, errInvalidConfig
	}
	if !cfg.IsAggregator && aggregateSubnetIDs != "" {
		fmt.Fprintln(stderr, "--aggregate-subnet-ids requires --is-aggregator")
		return cfg, errInvalidConfig
	}
	if err := resolveShadowRates(fs, &cfg, stderr); err != nil {
		return cfg, err
	}
	if (cfg.ExecutionEndpoint == "") != (cfg.ExecutionJWTSecret == "") {
		fmt.Fprintln(stderr, "--execution-endpoint and --execution-jwt-secret must be given together")
		return cfg, errInvalidConfig
	}
	if feeRecipient != "" {
		recipient, err := parseAddress(feeRecipient)
		if err != nil {
			fmt.Fprintf(stderr, "invalid --suggested-fee-recipient: %v\n", err)
			return cfg, errInvalidConfig
		}
		cfg.FeeRecipient = recipient
	}

	subnetIDs, err := parseAggregateSubnetIDs(aggregateSubnetIDs, stderr)
	if err != nil {
		return cfg, err
	}
	if err := validateAggregateSubnetIDs(subnetIDs, cfg.CommitteeCount, stderr); err != nil {
		return cfg, err
	}
	cfg.AggregateSubnetIDs = subnetIDs
	return cfg, nil
}

// shadowRateSpec ties each Shadow prover-rate flag to its env-var fallback and
// the config field it fills.
type shadowRateSpec struct {
	flag   string
	env    string
	target *float64
}

func shadowRateSpecs(cfg *config) []shadowRateSpec {
	return []shadowRateSpec{
		{"shadow-xmss-aggregate-signatures-rate", "GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE", &cfg.ShadowAggregateSignaturesRate},
		{"shadow-xmss-verify-signature-rate", "GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE", &cfg.ShadowVerifySignatureRate},
		{"shadow-xmss-verify-aggregated-signatures-rate", "GEAN_SHADOW_XMSS_VERIFY_AGGREGATED_SIGNATURES_RATE", &cfg.ShadowVerifyAggregatedSignaturesRate},
	}
}

// resolveShadowRates applies the GEAN_SHADOW_* env-var fallback for any prover-rate
// flag the user did not pass explicitly (precedence: flag > env > 0, matching the
// convention the other lean clients expose), then rejects negative rates. A Shadow
// harness can inject per-node rates via the environment without rewriting argv.
func resolveShadowRates(fs *flag.FlagSet, cfg *config, stderr io.Writer) error {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, sp := range shadowRateSpecs(cfg) {
		if !set[sp.flag] {
			if raw, ok := os.LookupEnv(sp.env); ok {
				v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
				if err != nil {
					fmt.Fprintf(stderr, "invalid %s=%q: %v\n", sp.env, raw, err)
					return errInvalidConfig
				}
				*sp.target = v
			}
		}
		if *sp.target < 0 {
			fmt.Fprintf(stderr, "--%s must not be negative (env %s)\n", sp.flag, sp.env)
			return errInvalidConfig
		}
	}
	return nil
}

// resolveCommitteeCount picks the effective attestation committee count.
// Precedence is the lean-network convention: an explicit
// --attestation-committee-count flag wins, otherwise the shared config.yaml
// ATTESTATION_COMMITTEE_COUNT, otherwise the flag default (the spec value).
// The loader already guarantees configCount, when present, is >= 1.
func resolveCommitteeCount(flagCount uint64, flagSet bool, configCount *uint64) uint64 {
	if flagSet {
		return flagCount
	}
	if configCount != nil {
		return *configCount
	}
	return flagCount
}

func validatePort(name string, port int, stderr io.Writer) error {
	if port < 0 || port > 65535 {
		fmt.Fprintf(stderr, "--%s must be between 0 and 65535\n", name)
		return errInvalidConfig
	}
	return nil
}

func parseAggregateSubnetIDs(raw string, stderr io.Writer) ([]uint64, error) {
	if raw == "" {
		return nil, nil
	}
	var ids []uint64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "invalid aggregate-subnet-id %q: %v\n", part, err)
			return nil, errInvalidConfig
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func validateAggregateSubnetIDs(ids []uint64, committeeCount uint64, stderr io.Writer) error {
	for _, id := range ids {
		if id >= committeeCount {
			fmt.Fprintf(stderr, "--aggregate-subnet-ids contains %d, want < --attestation-committee-count (%d)\n",
				id, committeeCount)
			return errInvalidConfig
		}
	}
	return nil
}

func (c config) paths() configPaths {
	return configPaths{
		config:     filepath.Join(c.ConfigDir, "config.yaml"),
		bootnodes:  filepath.Join(c.ConfigDir, "nodes.yaml"),
		validators: filepath.Join(c.ConfigDir, "annotated_validators.yaml"),
		keysDir:    filepath.Join(c.ConfigDir, "hash-sig-keys"),
	}
}

func (c config) apiAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.APIPort))
}

func (c config) metricsAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.MetricsPort))
}

// parseAddress decodes a 20-byte hex address, with or without a 0x prefix.
func parseAddress(raw string) ([types.AddressSize]byte, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(raw), "0x"), "0X")
	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		return [types.AddressSize]byte{}, fmt.Errorf("%q is not hex", raw)
	}
	if len(decoded) != types.AddressSize {
		return [types.AddressSize]byte{}, fmt.Errorf("%q is %d bytes, want %d", raw, len(decoded), types.AddressSize)
	}
	var address [types.AddressSize]byte
	copy(address[:], decoded)
	return address, nil
}
