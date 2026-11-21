package lean

import (
	"net"
	"path/filepath"
	"strconv"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/p2p/host"
)

// Config is what a lean node runs with.
type Config struct {
	// TestDriver starts only the hive test driver on the API address; no
	// other field but the HTTP address and API port applies.
	TestDriver     bool
	ConfigDir      string
	GossipPort     int
	HTTPAddr       string
	APIPort        int
	MetricsPort    int
	NodeKey        string
	NodeID         string
	CheckpointURL  string
	IsAggregator   bool
	ProverArena    bool
	CommitteeCount uint64
	// CommitteeCountSet reports whether CommitteeCount was set explicitly;
	// otherwise the network's config.yaml value applies.
	CommitteeCountSet  bool
	AggregateSubnetIDs []uint64
	DataDir            string

	// Discovery runs discv5 on DiscoveryPort and dials the peers it finds.
	// Off by default: no other lean client runs discv5 yet.
	Discovery     bool
	DiscoveryPort int
	// DiscoveryAdvertiseIP is the IP put in this node's ENR. Nil leaves it to
	// discv5, which learns it from what its peers see.
	DiscoveryAdvertiseIP net.IP

	// Shadow*Rate model XMSS prover cost for the Shadow network simulator, which
	// does not charge CPU time. Each is in signature-units per second; a
	// non-positive rate (the default) disables the delay, so real deployments are
	// unaffected.
	ShadowAggregateSignaturesRate        float64
	ShadowVerifySignatureRate            float64
	ShadowVerifyAggregatedSignaturesRate float64

	// Clock is the time source for the node's time-dependent decisions. Nil
	// means the system clock.
	Clock clock.Clock

	// Network, when set, carries the node's p2p traffic instead of QUIC on
	// GossipPort: an in-memory network for simulations. Discovery needs QUIC.
	Network host.Network
}

type configPaths struct {
	config     string
	bootnodes  string
	validators string
	keysDir    string
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

func (c Config) paths() configPaths {
	return configPaths{
		config:     filepath.Join(c.ConfigDir, "config.yaml"),
		bootnodes:  filepath.Join(c.ConfigDir, "nodes.yaml"),
		validators: filepath.Join(c.ConfigDir, "annotated_validators.yaml"),
		keysDir:    filepath.Join(c.ConfigDir, "hash-sig-keys"),
	}
}

func (c Config) apiAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.APIPort))
}

func (c Config) metricsAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.MetricsPort))
}
