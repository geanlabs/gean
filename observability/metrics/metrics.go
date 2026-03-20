package metrics

import (
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Histogram bucket presets from leanMetrics spec.
var (
	fastBuckets  = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1}
	stfBuckets   = []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4}
	reorgBuckets = []float64{1, 2, 3, 5, 7, 10, 20, 30, 50, 100}
)

// --- Node Info ---

var NodeInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "lean_node_info",
	Help: "Node information (always 1)",
}, []string{"name", "version"})

var NodeStartTime = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_node_start_time_seconds",
	Help: "Start timestamp",
})

// --- PQ Signature Metrics ---

var PQSigAttestationSigningTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_pq_sig_attestation_signing_time_seconds",
	Help:    "Time taken to sign an attestation",
	Buckets: fastBuckets,
})

var PQSigAttestationVerificationTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_pq_sig_attestation_verification_time_seconds",
	Help:    "Time taken to verify an attestation signature",
	Buckets: fastBuckets,
})

var PQSigAggregatedSignaturesTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_aggregated_signatures_total",
	Help: "Total number of aggregated signatures",
})

var PQSigAttestationsInAggregatedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_attestations_in_aggregated_signatures_total",
	Help: "Total number of attestations included into aggregated signatures",
})

var PQSigSignaturesBuildingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_pq_sig_aggregated_signatures_building_time_seconds",
	Help:    "Time taken to build aggregated attestation signatures",
	Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
})

var PQSigAggregatedVerificationTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_pq_sig_aggregated_signatures_verification_time_seconds",
	Help:    "Time taken to verify an aggregated attestation signature",
	Buckets: fastBuckets,
})

var PQSigAggregatedValidTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_aggregated_signatures_valid_total",
	Help: "Total number of valid aggregated signatures",
})

var PQSigAggregatedInvalidTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_aggregated_signatures_invalid_total",
	Help: "Total number of invalid aggregated signatures",
})

var PQSigAttestationSignaturesTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_attestation_signatures_total",
	Help: "Total number of individual attestation signatures",
})

var PQSigAttestationSignaturesValidTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_attestation_signatures_valid_total",
	Help: "Total number of valid individual attestation signatures",
})

var PQSigAttestationSignaturesInvalidTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_pq_sig_attestation_signatures_invalid_total",
	Help: "Total number of invalid individual attestation signatures",
})

// --- Fork-Choice ---

var HeadSlot = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_head_slot",
	Help: "Latest slot of the lean chain",
})

var CurrentSlot = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_current_slot",
	Help: "Current slot of the lean chain",
})

var SafeTargetSlot = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_safe_target_slot",
	Help: "Safe target slot",
})

var ForkChoiceBlockProcessingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_fork_choice_block_processing_time_seconds",
	Help:    "Time taken to process block in fork choice",
	Buckets: fastBuckets,
})

var AttestationsValid = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "lean_attestations_valid_total",
	Help: "Total number of valid attestations",
}, []string{"source"})

var AttestationsInvalid = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "lean_attestations_invalid_total",
	Help: "Total number of invalid attestations",
}, []string{"source"})

var AttestationValidationTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_attestation_validation_time_seconds",
	Help:    "Time taken to validate attestation",
	Buckets: fastBuckets,
})

var ForkChoiceReorgsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_fork_choice_reorgs_total",
	Help: "Total number of fork choice reorgs",
})

var ForkChoiceReorgDepth = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_fork_choice_reorg_depth",
	Help:    "Depth of fork choice reorgs (in blocks)",
	Buckets: reorgBuckets,
})

// --- State Transition ---

var LatestJustifiedSlot = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_latest_justified_slot",
	Help: "Latest justified slot",
})

var LatestFinalizedSlot = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_latest_finalized_slot",
	Help: "Latest finalized slot",
})

var FinalizationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "lean_finalizations_total",
	Help: "Total number of finalization attempts",
}, []string{"result"})

var StateTransitionTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_state_transition_time_seconds",
	Help:    "Time to process state transition",
	Buckets: stfBuckets,
})

var STFSlotsProcessed = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_state_transition_slots_processed_total",
	Help: "Total number of processed slots",
})

var STFSlotsProcessingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_state_transition_slots_processing_time_seconds",
	Help:    "Time taken to process slots",
	Buckets: fastBuckets,
})

var STFBlockProcessingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_state_transition_block_processing_time_seconds",
	Help:    "Time taken to process block",
	Buckets: fastBuckets,
})

var STFAttestationsProcessed = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "lean_state_transition_attestations_processed_total",
	Help: "Total number of processed attestations",
})

var STFAttestationsProcessingTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_state_transition_attestations_processing_time_seconds",
	Help:    "Time taken to process attestations",
	Buckets: fastBuckets,
})

// --- Validator ---

var ValidatorsCount = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_validators_count",
	Help: "Number of validators managed by a node",
})

// --- Devnet-3 Aggregator ---

var IsAggregator = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_is_aggregator",
	Help: "Whether the node is acting as an aggregator (1 = yes, 0 = no)",
})

var AttestationCommitteeCount = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_attestation_committee_count",
	Help: "Number of attestation committees",
})

var AttestationCommitteeSubnet = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_attestation_committee_subnet",
	Help: "Subnet ID assigned to this node's validators",
})

var CommitteeSignaturesAggregationTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_committee_signatures_aggregation_time_seconds",
	Help:    "Time taken to aggregate committee signatures",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
})

var GossipSignaturesCount = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_gossip_signatures",
	Help: "Number of gossip signatures in fork-choice store",
})

var LatestNewAggregatedPayloads = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_latest_new_aggregated_payloads",
	Help: "Number of new aggregated payload items",
})

var LatestKnownAggregatedPayloads = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_latest_known_aggregated_payloads",
	Help: "Number of known aggregated payload items",
})

// --- Network ---

var ConnectedPeers = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_connected_peers",
	Help: "Number of connected peers",
})

var PeerConnectionEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "lean_peer_connection_events_total",
	Help: "Total number of peer connection events",
}, []string{"direction", "result"})

var PeerDisconnectionEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "lean_peer_disconnection_events_total",
	Help: "Total number of peer disconnection events",
}, []string{"direction", "reason"})

func init() {
	prometheus.MustRegister(
		// Node info
		NodeInfo,
		NodeStartTime,
		// PQ signatures
		PQSigAttestationSigningTime,
		PQSigAttestationVerificationTime,
		PQSigAggregatedSignaturesTotal,
		PQSigAttestationsInAggregatedTotal,
		PQSigSignaturesBuildingTime,
		PQSigAggregatedVerificationTime,
		PQSigAggregatedValidTotal,
		PQSigAggregatedInvalidTotal,
		// Fork choice
		HeadSlot,
		CurrentSlot,
		SafeTargetSlot,
		ForkChoiceBlockProcessingTime,
		AttestationsValid,
		AttestationsInvalid,
		AttestationValidationTime,
		ForkChoiceReorgsTotal,
		ForkChoiceReorgDepth,
		// State transition
		LatestJustifiedSlot,
		LatestFinalizedSlot,
		FinalizationsTotal,
		StateTransitionTime,
		STFSlotsProcessed,
		STFSlotsProcessingTime,
		STFBlockProcessingTime,
		STFAttestationsProcessed,
		STFAttestationsProcessingTime,
		// Validator
		ValidatorsCount,
		// Devnet-3 aggregator
		IsAggregator,
		AttestationCommitteeCount,
		AttestationCommitteeSubnet,
		CommitteeSignaturesAggregationTime,
		GossipSignaturesCount,
		LatestNewAggregatedPayloads,
		LatestKnownAggregatedPayloads,
		// PQ attestation signatures
		PQSigAttestationSignaturesTotal,
		PQSigAttestationSignaturesValidTotal,
		PQSigAttestationSignaturesInvalidTotal,
		// Network
		ConnectedPeers,
		PeerConnectionEventsTotal,
		PeerDisconnectionEventsTotal,
	)

	// Pre-initialize vector counters to 0 to ensure they appear in metrics output
	// before any events occur, preventing "No data" in Grafana panels.
	for _, source := range []string{"gossip", "block", "subnet", "aggregation"} {
		AttestationsValid.WithLabelValues(source).Add(0)
		AttestationsInvalid.WithLabelValues(source).Add(0)
	}

	for _, dir := range []string{"inbound", "outbound"} {
		PeerConnectionEventsTotal.WithLabelValues(dir, "success").Add(0)
		PeerDisconnectionEventsTotal.WithLabelValues(dir, "remote_close").Add(0)
	}

	PQSigAttestationSignaturesTotal.Add(0)
	PQSigAttestationSignaturesValidTotal.Add(0)
	PQSigAttestationSignaturesInvalidTotal.Add(0)
}

// Serve starts the Prometheus metrics HTTP server on the given port.
func Serve(port int) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()
}
