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
	fastBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 1}
	stfBuckets  = []float64{0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4}
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

var AttestationValidationTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_attestation_validation_time_seconds",
	Help:    "Time taken to validate attestation",
	Buckets: fastBuckets,
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

var StateTransitionTime = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "lean_state_transition_time_seconds",
	Help:    "Time to process state transition",
	Buckets: stfBuckets,
})

// --- Validator ---

var ValidatorsCount = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_validators_count",
	Help: "Number of validators managed by a node",
})

// --- Network ---

var ConnectedPeers = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "lean_connected_peers",
	Help: "Number of connected peers",
})

func init() {
	prometheus.MustRegister(
		// Node info
		NodeInfo,
		NodeStartTime,
		// Fork choice
		HeadSlot,
		CurrentSlot,
		SafeTargetSlot,
		ForkChoiceBlockProcessingTime,
		AttestationsValid,
		AttestationValidationTime,
		// State transition
		LatestJustifiedSlot,
		LatestFinalizedSlot,
		StateTransitionTime,
		// Validator
		ValidatorsCount,
		// Network
		ConnectedPeers,
	)
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
