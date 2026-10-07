package metrics

import (
	"math"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCountCounterWrappersIgnoreNonPositiveValues(t *testing.T) {
	m := New(prometheus.NewRegistry())
	before := metricValue(t, m.metricAttestationsBufferEvicted)

	m.IncAttestationsBufferEvicted(-2)
	m.IncAttestationsBufferEvicted(0)
	if got := metricValue(t, m.metricAttestationsBufferEvicted); got != before {
		t.Fatalf("counter changed after nonpositive adds: got %v want %v", got, before)
	}

	m.IncAttestationsBufferEvicted(3)
	if got, want := metricValue(t, m.metricAttestationsBufferEvicted)-before, float64(3); got != want {
		t.Fatalf("counter delta=%v, want %v", got, want)
	}
}

func TestIncAggregatorSkippedUsesBoundedReasons(t *testing.T) {
	m := New(prometheus.NewRegistry())
	reasons := []string{
		AggregatorSkipNotAggregator,
		AggregatorSkipMissingState,
		AggregatorSkipSpawnFailed,
		AggregatorSkipOther,
	}
	for _, reason := range reasons {
		before := metricValue(t, m.metricAggregatorSkipped.WithLabelValues(reason))
		m.IncAggregatorSkipped(reason)
		if got := metricValue(t, m.metricAggregatorSkipped.WithLabelValues(reason)); got != before+1 {
			t.Fatalf("aggregator skipped %s=%v, want %v", reason, got, before+1)
		}
	}

	beforeOther := metricValue(t, m.metricAggregatorSkipped.WithLabelValues(AggregatorSkipOther))
	m.IncAggregatorSkipped("unexpected")
	if got := metricValue(t, m.metricAggregatorSkipped.WithLabelValues(AggregatorSkipOther)); got != beforeOther+1 {
		t.Fatalf("unexpected reason did not map to other: got %v want %v", got, beforeOther+1)
	}
}

func TestGaugeWrappersClampNegativeCounts(t *testing.T) {
	m := New(prometheus.NewRegistry())
	m.SetValidatorsCount(-1)
	if got := metricValue(t, m.metricValidatorsCount); got != 0 {
		t.Fatalf("validators gauge=%v, want 0", got)
	}

	m.SetGossipMeshPeers(-5)
	if got := metricValue(t, m.metricGossipMeshPeers); got != 0 {
		t.Fatalf("mesh peers gauge=%v, want 0", got)
	}

	m.SetConnectedPeers("", -3)
	if got := metricValue(t, m.metricConnectedPeers.WithLabelValues(unknownLabel)); got != 0 {
		t.Fatalf("connected peers gauge=%v, want 0", got)
	}

	m.SetConnectedPeers("", 4)
	if got := metricValue(t, m.metricConnectedPeers.WithLabelValues(unknownLabel)); got != 4 {
		t.Fatalf("connected peers gauge=%v, want 4", got)
	}
}

func TestSetIsAggregatorUsesBooleanGauge(t *testing.T) {
	m := New(prometheus.NewRegistry())
	m.SetIsAggregator(true)
	if got := metricValue(t, m.metricIsAggregator); got != 1 {
		t.Fatalf("aggregator gauge=%v, want 1", got)
	}

	m.SetIsAggregator(false)
	if got := metricValue(t, m.metricIsAggregator); got != 0 {
		t.Fatalf("aggregator gauge=%v, want 0", got)
	}
}

func TestSetSyncStatusActivatesSingleStatus(t *testing.T) {
	m := New(prometheus.NewRegistry())
	m.SetSyncStatus("syncing")
	assertSyncStatus(t, m, "idle", 0)
	assertSyncStatus(t, m, "syncing", 1)
	assertSyncStatus(t, m, "synced", 0)
	assertSyncStatus(t, m, unknownLabel, 0)

	m.SetSyncStatus("unexpected")
	assertSyncStatus(t, m, "idle", 0)
	assertSyncStatus(t, m, "syncing", 0)
	assertSyncStatus(t, m, "synced", 0)
	assertSyncStatus(t, m, unknownLabel, 1)
}

func TestLabelOrUnknownNormalizesDynamicLabels(t *testing.T) {
	if got := labelOrUnknown("  lodestar  "); got != "lodestar" {
		t.Fatalf("label=%q, want trimmed label", got)
	}
	if got := labelOrUnknown(" \t\n "); got != unknownLabel {
		t.Fatalf("blank label=%q, want unknown", got)
	}

	long := strings.Repeat("x", maxLabelRunes+10)
	if got := labelOrUnknown(long); len([]rune(got)) != maxLabelRunes {
		t.Fatalf("truncated label rune length=%d, want %d", len([]rune(got)), maxLabelRunes)
	}

	unicodeLong := strings.Repeat("é", maxLabelRunes+1)
	if got := labelOrUnknown(unicodeLong); len([]rune(got)) != maxLabelRunes || !strings.HasSuffix(got, "é") {
		t.Fatalf("unicode label=%q was not truncated on rune boundary", got)
	}
}

func TestHistogramWrappersIgnoreInvalidObservations(t *testing.T) {
	m := New(prometheus.NewRegistry())
	before := histogramCount(t, m.metricBlockProcessingTime)

	m.ObserveBlockProcessingTime(-1)
	m.ObserveBlockProcessingTime(math.NaN())
	m.ObserveBlockProcessingTime(math.Inf(1))
	if got := histogramCount(t, m.metricBlockProcessingTime); got != before {
		t.Fatalf("histogram count changed after invalid observations: got %d want %d", got, before)
	}

	m.ObserveBlockProcessingTime(0.25)
	if got := histogramCount(t, m.metricBlockProcessingTime); got != before+1 {
		t.Fatalf("histogram count after valid observation=%d, want %d", got, before+1)
	}
}

func TestSetNodeStartTimeIgnoresInvalidValues(t *testing.T) {
	m := New(prometheus.NewRegistry())
	m.SetNodeStartTime(100)
	before := metricValue(t, m.metricNodeStartTime)

	m.SetNodeStartTime(-1)
	m.SetNodeStartTime(math.NaN())
	m.SetNodeStartTime(math.Inf(1))
	if got := metricValue(t, m.metricNodeStartTime); got != before {
		t.Fatalf("node start time changed after invalid values: got %v want %v", got, before)
	}
}

func assertSyncStatus(t *testing.T, m *Metrics, status string, want float64) {
	t.Helper()
	if got := metricValue(t, m.metricNodeSyncStatus.WithLabelValues(status)); got != want {
		t.Fatalf("sync status %q=%v, want %v", status, got, want)
	}
}

func metricValue(t *testing.T, metric prometheus.Metric) float64 {
	t.Helper()

	var dtoMetric dto.Metric
	if err := metric.Write(&dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	if gauge := dtoMetric.GetGauge(); gauge != nil {
		return gauge.GetValue()
	}
	if counter := dtoMetric.GetCounter(); counter != nil {
		return counter.GetValue()
	}
	t.Fatal("metric has no gauge or counter value")
	return 0
}

func histogramCount(t *testing.T, metric prometheus.Metric) uint64 {
	t.Helper()

	var dtoMetric dto.Metric
	if err := metric.Write(&dtoMetric); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	histogram := dtoMetric.GetHistogram()
	if histogram == nil {
		t.Fatal("metric has no histogram value")
	}
	return histogram.GetSampleCount()
}
