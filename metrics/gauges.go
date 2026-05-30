package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics use the lean_ prefix.

var (
	metricHeadSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_head_slot", Help: "Latest head slot",
	})
	metricCurrentSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_current_slot", Help: "Current slot from wall clock",
	})
	metricSafeTargetSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_safe_target_slot", Help: "Safe target slot for attestation",
	})
	metricLatestJustifiedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_justified_slot", Help: "Latest justified checkpoint slot",
	})
	metricLatestFinalizedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_finalized_slot", Help: "Latest finalized checkpoint slot",
	})
	metricValidatorsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_validators_count", Help: "Number of validators managed by this node",
	})
	metricIsAggregator = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_is_aggregator", Help: "Whether this node is an aggregator (0 or 1)",
	})
	metricAttestationCommitteeCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_attestation_committee_count", Help: "Number of attestation committees/subnets",
	})
	// lean_gossip_signatures is the leanMetrics-standard name (data-source
	// flavored). It tracks the same pool that leanSpec renamed from
	// gossip_signatures -> attestation_signatures on the spec side; the
	// metric and field names move in opposite directions on purpose: the
	// metric is named for where the data comes from (gossip), the field for
	// what it contains (attestation signatures).
	metricGossipSignatures = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_gossip_signatures", Help: "Number of gossip signatures in fork-choice store",
	})
	metricLatestNewAggregatedPayloads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_new_aggregated_payloads", Help: "Number of new (pending) aggregated payloads",
	})
	metricLatestKnownAggregatedPayloads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_latest_known_aggregated_payloads", Help: "Number of known (active) aggregated payloads",
	})
	metricPendingAttestationsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_pending_attestations_total", Help: "Gossip attestations buffered awaiting an unknown head block",
	})
	metricNodeInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_node_info", Help: "Node information",
	}, []string{"name", "version"})
	metricNodeStartTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_node_start_time_seconds", Help: "Node start time as Unix timestamp",
	})
	metricTableBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_table_bytes", Help: "Estimated table size in bytes",
	}, []string{"table"})
	metricConnectedPeers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "lean_connected_peers", Help: "Number of connected peers",
	}, []string{"client"})
	metricAttestationCommitteeSubnet = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_attestation_committee_subnet", Help: "Node's attestation committee subnet",
	})
	metricGossipMeshPeers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_gossip_mesh_peers", Help: "Number of peers in the gossipsub mesh",
	})
)

func SetNodeInfo(name, version string) { metricNodeInfo.WithLabelValues(name, version).Set(1) }
func SetNodeStartTime(t float64)       { metricNodeStartTime.Set(t) }
func SetHeadSlot(s uint64)             { metricHeadSlot.Set(float64(s)) }
func SetCurrentSlot(s uint64)          { metricCurrentSlot.Set(float64(s)) }
func SetSafeTargetSlot(s uint64)       { metricSafeTargetSlot.Set(float64(s)) }
func SetLatestJustifiedSlot(s uint64)  { metricLatestJustifiedSlot.Set(float64(s)) }
func SetLatestFinalizedSlot(s uint64)  { metricLatestFinalizedSlot.Set(float64(s)) }
func SetValidatorsCount(n int)         { metricValidatorsCount.Set(float64(n)) }
func SetIsAggregator(b bool) {
	if b {
		metricIsAggregator.Set(1)
	} else {
		metricIsAggregator.Set(0)
	}
}
func SetAttestationCommitteeCount(n uint64) { metricAttestationCommitteeCount.Set(float64(n)) }
func SetGossipSignatures(n int)             { metricGossipSignatures.Set(float64(n)) }
func SetNewAggregatedPayloads(n int)        { metricLatestNewAggregatedPayloads.Set(float64(n)) }
func SetKnownAggregatedPayloads(n int)      { metricLatestKnownAggregatedPayloads.Set(float64(n)) }
func SetPendingAttestationsTotal(n int)     { metricPendingAttestationsTotal.Set(float64(n)) }
func SetTableBytes(table string, bytes uint64) {
	metricTableBytes.WithLabelValues(table).Set(float64(bytes))
}
func SetConnectedPeers(client string, n int) {
	metricConnectedPeers.WithLabelValues(client).Set(float64(n))
}
func SetAttestationCommitteeSubnet(n uint64) { metricAttestationCommitteeSubnet.Set(float64(n)) }
func SetGossipMeshPeers(n int)               { metricGossipMeshPeers.Set(float64(n)) }
