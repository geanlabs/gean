package metrics

func ObserveBlockProcessingTime(seconds float64) {
	observeNonNegative(metricBlockProcessingTime, seconds)
}
func ObservePqSigSigningTime(seconds float64) { observeNonNegative(metricPqSigSigningTime, seconds) }
func ObservePqSigVerificationTime(seconds float64) {
	observeNonNegative(metricPqSigVerificationTime, seconds)
}
func ObservePqSigAggBuildingTime(seconds float64) {
	observeNonNegative(metricPqSigAggBuildingTime, seconds)
}
func ObserveAggregationPrepTime(seconds float64) {
	observeNonNegative(metricAggregationPrepTime, seconds)
}
func ObserveForkChoiceReorgDepth(depth float64) {
	observeNonNegative(metricForkChoiceReorgDepth, depth)
}
func ObserveTickIntervalDuration(seconds float64) {
	observeNonNegative(metricTickIntervalDuration, seconds)
}
func ObserveSTFTime(seconds float64)           { observeNonNegative(metricSTFTime, seconds) }
func ObserveBlockBuildingTime(seconds float64) { observeNonNegative(metricBlockBuildingTime, seconds) }
func ObserveBlockAggregatedPayloads(n int) {
	observeNonNegative(metricBlockAggregatedPayloads, countValue(n))
}
func ObserveGossipBlockSize(bytes int) { observeNonNegative(metricGossipBlockSize, countValue(bytes)) }
func ObserveGossipAttestationSize(bytes int) {
	observeNonNegative(metricGossipAttestationSize, countValue(bytes))
}
func ObserveGossipAggregationSize(bytes int) {
	observeNonNegative(metricGossipAggregationSize, countValue(bytes))
}

func ObserveAttestationValidationTime(seconds float64) {
	observeNonNegative(metricAttestationValidationTime, seconds)
}

func ObserveAttestationsProductionTime(seconds float64) {
	observeNonNegative(metricAttestationsProductionTime, seconds)
}

func ObserveAggregationWorkerTotalTime(seconds float64) {
	observeNonNegative(metricAggregationWorkerTotalTime, seconds)
}

func ObserveBlockSignatureVerificationTime(seconds float64) {
	observeNonNegative(metricBlockSignatureVerificationTime, seconds)
}
