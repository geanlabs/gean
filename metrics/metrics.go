package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics records one node's metrics. Each instance registers its own
// collectors, so several nodes in one process keep separate observations. A
// nil *Metrics records nothing.
type Metrics struct {
	metricHeadSlot    prometheus.Gauge
	metricCurrentSlot prometheus.Gauge
	// metricTickAge is the stall detector. lean_tick_interval_duration_seconds is
	// a histogram observed *inside* onTick, so it records nothing at all while the
	// dispatch loop is blocked — the failure it should report makes it go quiet
	// rather than spike, and its top bucket is 1.6s besides. This gauge is written
	// from a separate goroutine and keeps climbing for as long as the loop is stuck.
	metricTickAge                                    prometheus.Gauge
	metricSafeTargetSlot                             prometheus.Gauge
	metricLatestJustifiedSlot                        prometheus.Gauge
	metricLatestFinalizedSlot                        prometheus.Gauge
	metricJustifiedSlot                              prometheus.Gauge
	metricFinalizedSlot                              prometheus.Gauge
	metricValidatorsCount                            prometheus.Gauge
	metricIsAggregator                               prometheus.Gauge
	metricAttestationCommitteeCount                  prometheus.Gauge
	metricGossipSignatures                           prometheus.Gauge
	metricLatestNewAggregatedPayloads                prometheus.Gauge
	metricLatestKnownAggregatedPayloads              prometheus.Gauge
	metricPendingAttestationsTotal                   prometheus.Gauge
	metricNodeInfo                                   *prometheus.GaugeVec
	metricNodeStartTime                              prometheus.Gauge
	metricConnectedPeers                             *prometheus.GaugeVec
	metricAttestationCommitteeSubnet                 prometheus.Gauge
	metricGossipMeshPeers                            prometheus.Gauge
	metricNodeSyncStatus                             *prometheus.GaugeVec
	metricAttestationAggregateCoverageValidators     *prometheus.GaugeVec
	metricAttestationAggregateCoverageSubnets        *prometheus.GaugeVec
	metricAttestationAggregateCoverageDiffValidators *prometheus.GaugeVec
	metricProvingQueueDepth                          *prometheus.GaugeVec
	metricProcessRSSBytes                            prometheus.Gauge
	metricTableBytes                                 *prometheus.GaugeVec
	metricAttestationsValid                          prometheus.Counter
	metricAttestationsInvalid                        prometheus.Counter
	metricAttestationsBufferEvicted                  prometheus.Counter
	metricAggregationDispatchDropped                 prometheus.Counter
	metricAggregatorSkipped                          *prometheus.CounterVec
	metricAggregationGroupSkipped                    *prometheus.CounterVec
	metricForkChoiceReorgs                           prometheus.Counter
	metricPqSigAggregatedSignaturesTotal             prometheus.Counter
	metricPqSigAggregatedValid                       prometheus.Counter
	metricPqSigAggregatedInvalid                     prometheus.Counter
	metricSTFSlotsProcessed                          prometheus.Counter
	metricSTFAttestationsProcessed                   prometheus.Counter
	metricPqSigAttestationsInAggregated              prometheus.Counter
	metricPqSigAttestationSigsTotal                  prometheus.Counter
	metricPqSigAttestationSigsValid                  prometheus.Counter
	metricPqSigAttestationSigsInvalid                prometheus.Counter
	metricFinalizationsTotal                         *prometheus.CounterVec
	metricPeerConnectionEvents                       *prometheus.CounterVec
	metricPeerDisconnectionEvents                    *prometheus.CounterVec
	metricBlockBuildingSuccess                       prometheus.Counter
	metricBlockBuildingFailures                      prometheus.Counter
	metricBlocksSkippedLag                           prometheus.Counter
	metricAttestationsSkippedLag                     prometheus.Counter
	metricProofOperations                            *prometheus.CounterVec
	metricReqRespTimeout                             *prometheus.CounterVec
	metricBlockProposalAttestationBuilds             prometheus.Counter
	metricBlockProcessingTime                        prometheus.Histogram
	metricAttestationValidationTime                  prometheus.Histogram
	metricPqSigSigningTime                           prometheus.Histogram
	metricAttestationsProductionTime                 prometheus.Histogram
	metricPqSigVerificationTime                      prometheus.Histogram
	metricPqSigAggBuildingTime                       prometheus.Histogram
	metricPqSigAggVerificationTime                   prometheus.Histogram
	metricCommitteeSignaturesAggregationTime         prometheus.Histogram
	metricAggregationPrepTime                        prometheus.Histogram
	metricAggregationWorkerTotalTime                 prometheus.Histogram
	metricBlockSignatureVerificationTime             prometheus.Histogram
	metricForkChoiceReorgDepth                       prometheus.Histogram
	metricSTFTime                                    prometheus.Histogram
	metricSTFSlotsTime                               prometheus.Histogram
	metricSTFBlockTime                               prometheus.Histogram
	metricSTFAttestationsTime                        prometheus.Histogram
	metricBlockBuildingTime                          prometheus.Histogram
	metricBlockBuildingPayloadAggregationTime        prometheus.Histogram
	metricBlockAggregatedPayloads                    prometheus.Histogram
	metricGossipBlockSize                            prometheus.Histogram
	metricGossipAttestationSize                      prometheus.Histogram
	metricGossipAggregationSize                      prometheus.Histogram
	metricTickIntervalDuration                       prometheus.Histogram
	// metricDispatchEventDuration times each case of the dispatch select, so a
	// slow handler can be attributed rather than only observed as a late tick.
	// Buckets run well past a slot: the point is to size a stall, and the
	// tick-interval histogram's 1.6s ceiling could not.
	metricDispatchEventDuration                *prometheus.HistogramVec
	metricProvingDuration                      *prometheus.HistogramVec
	metricProposalStageDuration                *prometheus.HistogramVec
	metricProofSize                            *prometheus.HistogramVec
	metricProofMergeComponents                 prometheus.Histogram
	metricReqRespRequestSize                   *prometheus.HistogramVec
	metricReqRespResponseChunkSize             *prometheus.HistogramVec
	metricBlockProposalAttestationDataSelected prometheus.Histogram
	metricBlockProposalAggregatesSelected      prometheus.Histogram
}

// New registers a node's collectors with reg. The gean binary passes
// prometheus.DefaultRegisterer; a simulation gives each node its own registry.
func New(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	return &Metrics{
		metricHeadSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_head_slot", Help: "Latest head slot",
		}),
		metricCurrentSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_current_slot", Help: "Current slot from wall clock",
		}),
		metricTickAge: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_tick_last_age_seconds", Help: "Seconds since the dispatch loop last began a tick",
		}),
		metricSafeTargetSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_safe_target_slot", Help: "Safe target slot for attestation",
		}),
		metricLatestJustifiedSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_latest_justified_slot", Help: "Latest justified checkpoint slot",
		}),
		metricLatestFinalizedSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_latest_finalized_slot", Help: "Latest finalized checkpoint slot",
		}),
		metricJustifiedSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_justified_slot", Help: "Current justified checkpoint slot",
		}),
		metricFinalizedSlot: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_finalized_slot", Help: "Current finalized checkpoint slot",
		}),
		metricValidatorsCount: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_validators_count", Help: "Number of validators managed by this node",
		}),
		metricIsAggregator: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_is_aggregator", Help: "Whether this node is an aggregator (0 or 1)",
		}),
		metricAttestationCommitteeCount: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_attestation_committee_count", Help: "Number of attestation committees/subnets",
		}),
		metricGossipSignatures: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_gossip_signatures", Help: "Number of gossip signatures in fork-choice store",
		}),
		metricLatestNewAggregatedPayloads: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_latest_new_aggregated_payloads", Help: "Number of new (pending) aggregated payloads",
		}),
		metricLatestKnownAggregatedPayloads: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_latest_known_aggregated_payloads", Help: "Number of known (active) aggregated payloads",
		}),
		metricPendingAttestationsTotal: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_pending_attestations_total", Help: "Gossip attestations buffered awaiting an unknown head block",
		}),
		metricNodeInfo: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_node_info", Help: "Node information",
		}, []string{"name", "version"}),
		metricNodeStartTime: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_node_start_time_seconds", Help: "Node start time as Unix timestamp",
		}),
		metricConnectedPeers: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_connected_peers", Help: "Number of connected peers",
		}, []string{"client"}),
		metricAttestationCommitteeSubnet: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_attestation_committee_subnet", Help: "Node's attestation committee subnet",
		}),
		metricGossipMeshPeers: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_gossip_mesh_peers", Help: "Number of peers in the gossipsub mesh",
		}),
		metricNodeSyncStatus: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_node_sync_status", Help: "Node sync status",
		}, []string{"status"}),
		metricAttestationAggregateCoverageValidators: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_attestation_aggregate_coverage_validators",
			Help: "Validator coverage in attestation aggregate reports, by section and subnet (subnet=combined is the section total)",
		}, []string{"section", "subnet"}),
		metricAttestationAggregateCoverageSubnets: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_attestation_aggregate_coverage_subnets",
			Help: "Number of covered subnets in attestation aggregate reports, by section",
		}, []string{"section"}),
		metricAttestationAggregateCoverageDiffValidators: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_attestation_aggregate_coverage_diff_validators",
			Help: "Validator coverage delta between block payloads and timely pre-merge payloads, by direction",
		}, []string{"direction"}),
		metricProvingQueueDepth: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_proving_queue_depth", Help: "Queued recursive proof work",
		}, []string{"operation"}),
		metricProcessRSSBytes: f.NewGauge(prometheus.GaugeOpts{
			Name: "lean_node_rss_bytes", Help: "Process resident set size in bytes (includes prover memory outside the Go heap)",
		}),
		metricTableBytes: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lean_table_bytes", Help: "Estimated on-disk byte size of a storage table",
		}, []string{"table"}),
		metricAttestationsValid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_attestations_valid_total", Help: "Total valid attestations processed",
		}),
		metricAttestationsInvalid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_attestations_invalid_total", Help: "Total invalid attestations rejected",
		}),
		metricAttestationsBufferEvicted: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_attestations_buffer_evicted_total", Help: "Pending attestations dropped due to per-root FIFO overflow",
		}),
		metricAggregationDispatchDropped: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_aggregation_dispatch_dropped_total", Help: "Interval-2 aggregation dispatches dropped because the worker was still busy with the previous slot",
		}),
		metricAggregatorSkipped: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_aggregator_skipped_total",
			Help: "Aggregation cycles skipped by reason",
		}, []string{"reason"}),
		metricAggregationGroupSkipped: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_aggregation_groups_skipped_total",
			Help: "Aggregation groups dropped inside a session by reason",
		}, []string{"reason"}),
		metricForkChoiceReorgs: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_fork_choice_reorgs_total", Help: "Total fork choice reorgs",
		}),
		metricPqSigAggregatedSignaturesTotal: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_aggregated_signatures_total", Help: "Total aggregated signature proofs produced",
		}),
		metricPqSigAggregatedValid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_aggregated_signatures_valid_total", Help: "Total valid aggregated signature verifications",
		}),
		metricPqSigAggregatedInvalid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_aggregated_signatures_invalid_total", Help: "Total invalid aggregated signature verifications",
		}),
		metricSTFSlotsProcessed: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_state_transition_slots_processed_total", Help: "Total number of processed slots",
		}),
		metricSTFAttestationsProcessed: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_state_transition_attestations_processed_total", Help: "Total number of processed attestations",
		}),
		metricPqSigAttestationsInAggregated: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_attestations_in_aggregated_signatures_total", Help: "Total attestations included in aggregated proofs",
		}),
		metricPqSigAttestationSigsTotal: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_attestation_signatures_total", Help: "Total individual attestation signatures processed",
		}),
		metricPqSigAttestationSigsValid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_attestation_signatures_valid_total", Help: "Total valid individual attestation signatures",
		}),
		metricPqSigAttestationSigsInvalid: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_pq_sig_attestation_signatures_invalid_total", Help: "Total invalid individual attestation signatures",
		}),
		metricFinalizationsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_finalizations_total", Help: "Total number of finalization attempts",
		}, []string{"result"}),
		metricPeerConnectionEvents: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_peer_connection_events_total", Help: "Total peer connection events",
		}, []string{"direction", "result"}),
		metricPeerDisconnectionEvents: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_peer_disconnection_events_total", Help: "Total peer disconnection events",
		}, []string{"direction", "reason"}),
		metricBlockBuildingSuccess: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_block_building_success_total", Help: "Successful block builds",
		}),
		metricBlockBuildingFailures: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_block_building_failures_total", Help: "Failed block builds",
		}),
		metricBlocksSkippedLag: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_node_blocks_skipped_lag_total",
			Help: "Block proposals skipped because the local view was too stale",
		}),
		metricAttestationsSkippedLag: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_node_attestations_skipped_lag_total",
			Help: "Attestation batches skipped because the local view was too stale",
		}),
		metricProofOperations: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_proof_operations_total", Help: "Recursive proof operations by result",
		}, []string{"operation", "result"}),
		metricReqRespTimeout: f.NewCounterVec(prometheus.CounterOpts{
			Name: "lean_p2p_reqresp_timeout_total",
			Help: "Req/resp stream operations aborted by the idle deadline, by protocol and direction",
		}, []string{"protocol", "direction"}),
		metricBlockProposalAttestationBuilds: f.NewCounter(prometheus.CounterOpts{
			Name: "lean_block_proposal_attestation_builds_total",
			Help: "Payloads selected into the proposal during greedy attestation planning",
		}),
		metricBlockProcessingTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_fork_choice_block_processing_time_seconds",
			Help:    "Time to process a block",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1, 1.25, 1.5, 2, 4},
		}),
		metricAttestationValidationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_attestation_validation_time_seconds",
			Help:    "Time to validate attestation data",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricPqSigSigningTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_pq_sig_attestation_signing_time_seconds",
			Help:    "Time to sign an attestation",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricAttestationsProductionTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_attestations_production_time_seconds",
			Help:    "Time taken to produce attestation",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
		}),
		metricPqSigVerificationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_pq_sig_attestation_verification_time_seconds",
			Help:    "Time to verify an individual attestation signature",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricPqSigAggBuildingTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_pq_sig_aggregated_signatures_building_time_seconds",
			Help:    "Time to build an aggregated signature proof",
			Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
		}),
		metricPqSigAggVerificationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_pq_sig_aggregated_signatures_verification_time_seconds",
			Help:    "Time to verify an aggregated attestation signature",
			Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 4},
		}),
		metricCommitteeSignaturesAggregationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_committee_signatures_aggregation_time_seconds",
			Help:    "Time taken to aggregate committee signatures",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 0.75, 1, 2, 3, 4},
		}),
		metricAggregationPrepTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_aggregation_prep_time_seconds",
			Help:    "Per-aggregate prep time",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricAggregationWorkerTotalTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_aggregation_worker_total_time_seconds",
			Help:    "End-to-end aggregation worker pass",
			Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4},
		}),
		metricBlockSignatureVerificationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_signature_verification_time_seconds",
			Help:    "Time to verify all signatures for an incoming block",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
		}),
		metricForkChoiceReorgDepth: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_fork_choice_reorg_depth",
			Help:    "Depth of fork choice reorgs",
			Buckets: []float64{1, 2, 3, 5, 7, 10, 20, 30, 50, 100},
		}),
		metricSTFTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_state_transition_time_seconds",
			Help:    "Time to process full state transition",
			Buckets: []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4},
		}),
		metricSTFSlotsTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_state_transition_slots_processing_time_seconds",
			Help:    "Time taken to process slots",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricSTFBlockTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_state_transition_block_processing_time_seconds",
			Help:    "Time taken to process block",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricSTFAttestationsTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_state_transition_attestations_processing_time_seconds",
			Help:    "Time taken to process attestations",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1},
		}),
		metricBlockBuildingTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_building_time_seconds",
			Help:    "Time to build a block",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1},
		}),
		metricBlockBuildingPayloadAggregationTime: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_building_payload_aggregation_time_seconds",
			Help:    "Time taken to build aggregated_payloads during block building",
			Buckets: []float64{0.1, 0.25, 0.5, 0.75, 1, 2, 3, 4},
		}),
		metricBlockAggregatedPayloads: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_aggregated_payloads",
			Help:    "Number of aggregated_payloads in a block",
			Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128},
		}),
		metricGossipBlockSize: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_gossip_block_size_bytes",
			Help:    "Bytes size of a gossip block message",
			Buckets: []float64{10000, 50000, 100000, 250000, 500000, 1000000, 2000000, 5000000},
		}),
		metricGossipAttestationSize: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_gossip_attestation_size_bytes",
			Help:    "Bytes size of a gossip attestation message",
			Buckets: []float64{512, 1024, 2048, 4096, 8192, 16384},
		}),
		metricGossipAggregationSize: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_gossip_aggregation_size_bytes",
			Help:    "Bytes size of a gossip aggregated attestation message",
			Buckets: []float64{1024, 4096, 16384, 65536, 131072, 262144, 524288, 1048576},
		}),
		metricTickIntervalDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_tick_interval_duration_seconds",
			Help:    "Elapsed time between clock ticks in seconds",
			Buckets: []float64{0.4, 0.6, 0.75, 0.8, 0.805, 0.81, 0.815, 0.82, 0.825, 0.85, 0.9, 1.0, 1.2, 1.6},
		}),
		metricDispatchEventDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_dispatch_event_duration_seconds",
			Help:    "Time the dispatch loop spent handling one event, by event kind",
			Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.25, 0.5, 0.8, 1.6, 4, 10, 30, 120, 600},
		}, []string{"event"}),
		metricProvingDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_proving_duration_seconds",
			Help:    "Recursive proof operation duration",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 4, 8},
		}, []string{"operation"}),
		metricProposalStageDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_proposal_stage_duration_seconds",
			Help:    "Proposal gate wait and native proof stage wall time, including failed attempts",
			Buckets: []float64{0.001, 0.01, 0.1, 0.25, 0.5, 1, 2, 4, 8, 16, 32, 64},
		}, []string{"stage"}),
		metricProofSize: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_proof_size_bytes",
			Help:    "Serialized aggregate proof size",
			Buckets: []float64{1024, 4096, 16384, 65536, 131072, 262144, 524288},
		}, []string{"type"}),
		metricProofMergeComponents: f.NewHistogram(prometheus.HistogramOpts{
			Name: "lean_proof_merge_components", Help: "Type-1 components merged into a Type-2 proof",
			Buckets: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}),
		metricReqRespRequestSize: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_reqresp_request_size_bytes",
			Help:    "On-wire bytes of a req/resp request frame",
			Buckets: []float64{64, 128, 256, 512, 1024, 4096, 16384, 65536},
		}, []string{"protocol"}),
		metricReqRespResponseChunkSize: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lean_reqresp_response_chunk_size_bytes",
			Help:    "On-wire bytes of a single req/resp response frame",
			Buckets: []float64{128, 1024, 10000, 100000, 500000, 1000000, 5000000, 10000000},
		}, []string{"protocol"}),
		metricBlockProposalAttestationDataSelected: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_proposal_attestation_data_selected",
			Help:    "Distinct AttestationData entries placed in the proposal block body",
			Buckets: []float64{0, 1, 2, 4, 8, 16, 32},
		}),
		metricBlockProposalAggregatesSelected: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "lean_block_proposal_aggregates_selected",
			Help:    "Aggregated signature proofs selected for the proposal",
			Buckets: []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
		}),
	}
}
