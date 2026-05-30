package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
	metricAggregationDispatchDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_aggregation_dispatch_dropped_total", Help: "Interval-2 aggregation dispatches dropped because the worker was still busy with the previous slot",
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
	metricBlockBuildingSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_block_building_success_total", Help: "Successful block builds",
	})
	metricBlockBuildingFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_block_building_failures_total", Help: "Failed block builds",
	})
)

// Duty-gate skip counters (leanSpec PR #708).
//
// Increment once per slot the duty gate denied production. The node-local
// prefix matches gean's convention for per-node state (lean_node_*) rather
// than the chain-level lean_* prefix because gating is informative and may
// differ across clients without breaking consensus.
var (
	metricBlocksSkippedLag = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_node_blocks_skipped_lag_total",
		Help: "Block proposals skipped because the local view was too stale (leanSpec PR #708 duty gate).",
	})
	metricAttestationsSkippedLag = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_node_attestations_skipped_lag_total",
		Help: "Attestation batches skipped because the local view was too stale (leanSpec PR #708 duty gate).",
	})
)

func IncAttestationsValid(n uint64)          { metricAttestationsValid.Add(float64(n)) }
func IncAttestationsInvalid()                { metricAttestationsInvalid.Inc() }
func IncAttestationsBufferEvicted(n int)     { metricAttestationsBufferEvicted.Add(float64(n)) }
func IncForkChoiceReorgs()                   { metricForkChoiceReorgs.Inc() }
func IncBlocksSkippedLag()                   { metricBlocksSkippedLag.Inc() }
func IncAttestationsSkippedLag()             { metricAttestationsSkippedLag.Inc() }
func IncPqSigAggregatedTotal()               { metricPqSigAggregatedSignaturesTotal.Inc() }
func IncPqSigAttestationsInAggregated(n int) { metricPqSigAttestationsInAggregated.Add(float64(n)) }
func IncPqSigAggregatedValid()               { metricPqSigAggregatedValid.Inc() }
func IncPqSigAggregatedInvalid()             { metricPqSigAggregatedInvalid.Inc() }
func IncPqSigAttestationSigsTotal()          { metricPqSigAttestationSigsTotal.Inc() }
func IncPqSigAttestationSigsValid()          { metricPqSigAttestationSigsValid.Inc() }
func IncPqSigAttestationSigsInvalid()        { metricPqSigAttestationSigsInvalid.Inc() }
func IncAggregationDispatchDropped()         { metricAggregationDispatchDropped.Inc() }
func IncFinalization(result string)          { metricFinalizationsTotal.WithLabelValues(result).Inc() }
func IncSTFSlotsProcessed(n uint64)          { metricSTFSlotsProcessed.Add(float64(n)) }
func IncSTFAttestationsProcessed(n uint64)   { metricSTFAttestationsProcessed.Add(float64(n)) }
func IncPeerConnection(direction, result string) {
	metricPeerConnectionEvents.WithLabelValues(direction, result).Inc()
}
func IncPeerDisconnection(direction, reason string) {
	metricPeerDisconnectionEvents.WithLabelValues(direction, reason).Inc()
}
func IncBlockBuildingSuccess()  { metricBlockBuildingSuccess.Inc() }
func IncBlockBuildingFailures() { metricBlockBuildingFailures.Inc() }
