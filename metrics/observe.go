package metrics

func (m *Metrics) ObserveBlockProcessingTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockProcessingTime, seconds)
}
func (m *Metrics) ObservePqSigSigningTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricPqSigSigningTime, seconds)
}
func (m *Metrics) ObservePqSigVerificationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricPqSigVerificationTime, seconds)
}
func (m *Metrics) ObservePqSigAggBuildingTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricPqSigAggBuildingTime, seconds)
}
func (m *Metrics) ObservePqSigAggVerificationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricPqSigAggVerificationTime, seconds)
}
func (m *Metrics) ObserveCommitteeSignaturesAggregationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricCommitteeSignaturesAggregationTime, seconds)
}
func (m *Metrics) ObserveAggregationPrepTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricAggregationPrepTime, seconds)
}
func (m *Metrics) ObserveForkChoiceReorgDepth(depth float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricForkChoiceReorgDepth, depth)
}
func (m *Metrics) ObserveTickIntervalDuration(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricTickIntervalDuration, seconds)
}

// ObserveDispatchEvent records how long one dispatch-loop event took.
func (m *Metrics) ObserveDispatchEvent(event string, seconds float64) {
	if m == nil {
		return
	}
	if seconds < 0 {
		return
	}
	m.metricDispatchEventDuration.WithLabelValues(event).Observe(seconds)
}

func (m *Metrics) ObserveSTFTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricSTFTime, seconds)
}
func (m *Metrics) ObserveSTFSlotsTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricSTFSlotsTime, seconds)
}
func (m *Metrics) ObserveSTFBlockTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricSTFBlockTime, seconds)
}
func (m *Metrics) ObserveSTFAttestationsTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricSTFAttestationsTime, seconds)
}
func (m *Metrics) ObserveBlockBuildingTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockBuildingTime, seconds)
}
func (m *Metrics) ObserveBlockBuildingPayloadAggregationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockBuildingPayloadAggregationTime, seconds)
}
func (m *Metrics) ObserveBlockAggregatedPayloads(n int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockAggregatedPayloads, countValue(n))
}
func (m *Metrics) ObserveGossipBlockSize(bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricGossipBlockSize, countValue(bytes))
}
func (m *Metrics) ObserveGossipAttestationSize(bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricGossipAttestationSize, countValue(bytes))
}
func (m *Metrics) ObserveGossipAggregationSize(bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricGossipAggregationSize, countValue(bytes))
}

func (m *Metrics) ObserveAttestationValidationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricAttestationValidationTime, seconds)
}

func (m *Metrics) ObserveAttestationsProductionTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricAttestationsProductionTime, seconds)
}

func (m *Metrics) ObserveAggregationWorkerTotalTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricAggregationWorkerTotalTime, seconds)
}

func (m *Metrics) ObserveBlockSignatureVerificationTime(seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockSignatureVerificationTime, seconds)
}

func (m *Metrics) ObserveProvingDuration(operation string, seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricProvingDuration.WithLabelValues(labelOrUnknown(operation)), seconds)
}

func (m *Metrics) ObserveProofSize(proofType string, bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricProofSize.WithLabelValues(labelOrUnknown(proofType)), countValue(bytes))
}

func (m *Metrics) ObserveProofMergeComponents(n int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricProofMergeComponents, countValue(n))
}

func (m *Metrics) ObserveReqRespRequestSize(protocol string, bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricReqRespRequestSize.WithLabelValues(labelOrUnknown(protocol)), countValue(bytes))
}

func (m *Metrics) ObserveReqRespResponseChunkSize(protocol string, bytes int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricReqRespResponseChunkSize.WithLabelValues(labelOrUnknown(protocol)), countValue(bytes))
}

func (m *Metrics) ObserveBlockProposalAttestationDataSelected(n int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockProposalAttestationDataSelected, countValue(n))
}

func (m *Metrics) ObserveBlockProposalAggregatesSelected(n int) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricBlockProposalAggregatesSelected, countValue(n))
}

func (m *Metrics) ObserveProposalStageDuration(stage string, seconds float64) {
	if m == nil {
		return
	}
	observeNonNegative(m.metricProposalStageDuration.WithLabelValues(labelOrUnknown(stage)), seconds)
}
