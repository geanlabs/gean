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
	metricBlocksSkippedLag = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_node_blocks_skipped_lag_total",
		Help: "Block proposals skipped because the local view was too stale",
	})
	metricAttestationsSkippedLag = promauto.NewCounter(prometheus.CounterOpts{
		Name: "lean_node_attestations_skipped_lag_total",
		Help: "Attestation batches skipped because the local view was too stale",
	})
)
