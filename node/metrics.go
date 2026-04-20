package node

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics use lean_ prefix rs.

// --- Gauges ---

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
	metricAttestationSignatures = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "lean_attestation_signatures", Help: "Number of attestation signature entries",
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
)

// --- Counters ---

var (
	metricAttestationsValid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_attestations_valid_total", Help: "Total valid attestations processed",
	})
	metricAttestationsInvalid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_attestations_invalid_total", Help: "Total invalid attestations rejected",
	})
	metricAttestationsBufferEvicted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_attestations_buffer_evicted_total", Help: "Pending attestations dropped due to per-root FIFO overflow",
	})
	metricForkChoiceReorgs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_fork_choice_reorgs_total", Help: "Total fork choice reorgs",
	})
	metricPqSigAggregatedSignaturesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_aggregated_signatures_total", Help: "Total aggregated signature proofs produced",
	})
	metricPqSigAttestationsInAggregated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_attestations_in_aggregated_signatures_total", Help: "Total attestations included in aggregated proofs",
	})
	metricPqSigAggregatedValid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_aggregated_signatures_valid_total", Help: "Total valid aggregated signature verifications",
	})
	metricPqSigAggregatedInvalid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_aggregated_signatures_invalid_total", Help: "Total invalid aggregated signature verifications",
	})
	metricPqSigAttestationSigsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_attestation_signatures_total", Help: "Total individual attestation signatures processed",
	})
	metricPqSigAttestationSigsValid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_attestation_signatures_valid_total", Help: "Total valid individual attestation signatures",
	})
	metricPqSigAttestationSigsInvalid = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_pq_sig_attestation_signatures_invalid_total", Help: "Total invalid individual attestation signatures",
	})
	metricFinalizationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lean_finalizations_total", Help: "Total number of finalization attempts",
	}, []string{"result"})
	metricSTFSlotsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_state_transition_slots_processed_total", Help: "Total number of processed slots",
	})
	metricSTFAttestationsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_state_transition_attestations_processed_total", Help: "Total number of processed attestations",
	})
	metricPeerConnectionEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lean_peer_connection_events_total", Help: "Total peer connection events",
	}, []string{"direction", "result"})
	metricPeerDisconnectionEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "lean_peer_disconnection_events_total", Help: "Total peer disconnection events",
	}, []string{"direction", "reason"})
)

// --- Histograms ---

var (
	metricBlockProcessingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_fork_choice_block_processing_time_seconds",
		Help:    "Time to process a block",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1, 1.25, 1.5, 2, 4},
	})
	metricAttestationValidationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_attestation_validation_time_seconds",
		Help:    "Time to validate attestation data",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricCommitteeAggregationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_committee_signatures_aggregation_time_seconds",
		Help:    "Time to aggregate committee signatures",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
	})
	metricPqSigSigningTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_attestation_signing_time_seconds",
		Help:    "Time to sign an attestation",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricAttestationsProductionTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_attestations_production_time_seconds",
		Help:    "Time taken to produce attestation",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
	})
	metricPqSigVerificationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_attestation_verification_time_seconds",
		Help:    "Time to verify an individual attestation signature",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricPqSigAggBuildingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_aggregated_signatures_building_time_seconds",
		Help:    "Time to build an aggregated signature proof",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
	})
	metricPqSigAggVerificationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_pq_sig_aggregated_signatures_verification_time_seconds",
		Help:    "Time to verify an aggregated signature proof",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
	})
	metricForkChoiceReorgDepth = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_fork_choice_reorg_depth",
		Help:    "Depth of fork choice reorgs",
		Buckets: []float64{1, 2, 3, 5, 7, 10, 20, 30, 50, 100},
	})
	metricSTFTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_time_seconds",
		Help:    "Time to process full state transition",
		Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4},
	})
	metricSTFSlotsTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_slots_processing_time_seconds",
		Help:    "Time to process slots",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricSTFBlockTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_block_processing_time_seconds",
		Help:    "Time to process block in state transition",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricSTFAttestationsTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_state_transition_attestations_processing_time_seconds",
		Help:    "Time to process attestations",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
	})
	metricBlockBuildingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_building_time_seconds",
		Help:    "Time to build a block",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
	})
	metricBlockBuildingPayloadAggregationTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_building_payload_aggregation_time_seconds",
		Help:    "Time to build aggregated_payloads during block building",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 2, 3, 4},
	})
	metricBlockAggregatedPayloads = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_block_aggregated_payloads",
		Help:    "Number of aggregated_payloads in a block",
		Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128},
	})
	metricGossipBlockSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_block_size_bytes",
		Help:    "Bytes size of a gossip block message",
		Buckets: []float64{10000, 50000, 100000, 250000, 500000, 1000000, 2000000, 5000000},
	})
	metricGossipAttestationSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_attestation_size_bytes",
		Help:    "Bytes size of a gossip attestation message",
		Buckets: []float64{512, 1024, 2048, 4096, 8192, 16384},
	})
	metricGossipAggregationSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "lean_gossip_aggregation_size_bytes",
		Help:    "Bytes size of a gossip aggregated attestation message",
		Buckets: []float64{1024, 4096, 16384, 65536, 131072, 262144, 524288, 1048576},
	})
)

// --- Counters for block production ---

var (
	metricBlockBuildingSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_block_building_success_total", Help: "Successful block builds",
	})
	metricBlockBuildingFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_block_building_failures_total", Help: "Failed block builds",
	})
)

// --- Sync status gauge ---

var metricNodeSyncStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "lean_node_sync_status", Help: "Node sync status (one of idle/syncing/synced is set to 1)",
}, []string{"status"})

// --- Public update functions ---

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
func SetAttestationSignatures(n int)        { metricAttestationSignatures.Set(float64(n)) }
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

func IncAttestationsValid(n uint64)          { metricAttestationsValid.Add(float64(n)) }
func IncAttestationsInvalid()                { metricAttestationsInvalid.Inc() }
func IncAttestationsBufferEvicted(n int)     { metricAttestationsBufferEvicted.Add(float64(n)) }
func IncForkChoiceReorgs()                   { metricForkChoiceReorgs.Inc() }
func IncPqSigAggregatedTotal()               { metricPqSigAggregatedSignaturesTotal.Inc() }
func IncPqSigAttestationsInAggregated(n int) { metricPqSigAttestationsInAggregated.Add(float64(n)) }
func IncPqSigAggregatedValid()               { metricPqSigAggregatedValid.Inc() }
func IncPqSigAggregatedInvalid()             { metricPqSigAggregatedInvalid.Inc() }
func IncPqSigAttestationSigsTotal()          { metricPqSigAttestationSigsTotal.Inc() }
func IncPqSigAttestationSigsValid()          { metricPqSigAttestationSigsValid.Inc() }
func IncPqSigAttestationSigsInvalid()        { metricPqSigAttestationSigsInvalid.Inc() }

func ObserveBlockProcessingTime(seconds float64) { metricBlockProcessingTime.Observe(seconds) }
func ObserveAttestationValidationTime(seconds float64) {
	metricAttestationValidationTime.Observe(seconds)
}
func ObserveCommitteeAggregationTime(seconds float64) {
	metricCommitteeAggregationTime.Observe(seconds)
}
func ObservePqSigSigningTime(seconds float64) { metricPqSigSigningTime.Observe(seconds) }
func ObserveAttestationsProductionTime(seconds float64) {
	metricAttestationsProductionTime.Observe(seconds)
}
func ObservePqSigVerificationTime(seconds float64) { metricPqSigVerificationTime.Observe(seconds) }
func ObservePqSigAggBuildingTime(seconds float64)  { metricPqSigAggBuildingTime.Observe(seconds) }
func ObservePqSigAggVerificationTime(seconds float64) {
	metricPqSigAggVerificationTime.Observe(seconds)
}
func ObserveForkChoiceReorgDepth(depth float64) { metricForkChoiceReorgDepth.Observe(depth) }

func IncFinalization(result string)        { metricFinalizationsTotal.WithLabelValues(result).Inc() }
func IncSTFSlotsProcessed(n uint64)        { metricSTFSlotsProcessed.Add(float64(n)) }
func IncSTFAttestationsProcessed(n uint64) { metricSTFAttestationsProcessed.Add(float64(n)) }
func IncPeerConnection(direction, result string) {
	metricPeerConnectionEvents.WithLabelValues(direction, result).Inc()
}
func IncPeerDisconnection(direction, reason string) {
	metricPeerDisconnectionEvents.WithLabelValues(direction, reason).Inc()
}
func ObserveSTFTime(seconds float64)             { metricSTFTime.Observe(seconds) }
func ObserveSTFSlotsTime(seconds float64)        { metricSTFSlotsTime.Observe(seconds) }
func ObserveSTFBlockTime(seconds float64)        { metricSTFBlockTime.Observe(seconds) }
func ObserveSTFAttestationsTime(seconds float64) { metricSTFAttestationsTime.Observe(seconds) }

// Block production observers/counters.
func ObserveBlockBuildingTime(seconds float64) { metricBlockBuildingTime.Observe(seconds) }
func ObserveBlockBuildingPayloadAggregationTime(seconds float64) {
	metricBlockBuildingPayloadAggregationTime.Observe(seconds)
}
func ObserveBlockAggregatedPayloads(n int) { metricBlockAggregatedPayloads.Observe(float64(n)) }
func IncBlockBuildingSuccess()             { metricBlockBuildingSuccess.Inc() }
func IncBlockBuildingFailures()            { metricBlockBuildingFailures.Inc() }

// Network gossip size observers.
func ObserveGossipBlockSize(bytes int)       { metricGossipBlockSize.Observe(float64(bytes)) }
func ObserveGossipAttestationSize(bytes int) { metricGossipAttestationSize.Observe(float64(bytes)) }
func ObserveGossipAggregationSize(bytes int) { metricGossipAggregationSize.Observe(float64(bytes)) }

// SetSyncStatus sets the active sync status to 1 and the others to 0.
// Valid values: "idle", "syncing", "synced".
func SetSyncStatus(status string) {
	for _, s := range []string{"idle", "syncing", "synced"} {
		if s == status {
			metricNodeSyncStatus.WithLabelValues(s).Set(1)
		} else {
			metricNodeSyncStatus.WithLabelValues(s).Set(0)
		}
	}
}
