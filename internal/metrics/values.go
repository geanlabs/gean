package metrics

import (
	"math"

	"github.com/prometheus/client_golang/prometheus"
)

func boolValue(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func countValue(n int) float64 {
	if n < 0 {
		return 0
	}
	return float64(n)
}

func addCount(counter prometheus.Counter, n int) {
	if counter == nil || n <= 0 {
		return
	}
	counter.Add(float64(n))
}

func addUint(counter prometheus.Counter, n uint64) {
	if counter == nil || n == 0 {
		return
	}
	counter.Add(float64(n))
}

func observeNonNegative(observer prometheus.Observer, value float64) {
	if observer == nil || !isNonNegativeFinite(value) {
		return
	}
	observer.Observe(value)
}

func setNonNegative(gauge prometheus.Gauge, value float64) {
	if gauge == nil || !isNonNegativeFinite(value) {
		return
	}
	gauge.Set(value)
}

func isNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
