package metrics

func (m *Metrics) IncAttestationsValid(n uint64) {
	if m == nil {
		return
	}
	addUint(m.metricAttestationsValid, n)
}
func (m *Metrics) IncAttestationsInvalid() {
	if m == nil {
		return
	}
	m.metricAttestationsInvalid.Inc()
}
func (m *Metrics) IncAttestationsBufferEvicted(n int) {
	if m == nil {
		return
	}
	addCount(m.metricAttestationsBufferEvicted, n)
}
func (m *Metrics) IncForkChoiceReorgs() {
	if m == nil {
		return
	}
	m.metricForkChoiceReorgs.Inc()
}
func (m *Metrics) IncBlocksSkippedLag() {
	if m == nil {
		return
	}
	m.metricBlocksSkippedLag.Inc()
}
func (m *Metrics) IncAttestationsSkippedLag() {
	if m == nil {
		return
	}
	m.metricAttestationsSkippedLag.Inc()
}
func (m *Metrics) IncPqSigAggregatedTotal() {
	if m == nil {
		return
	}
	m.metricPqSigAggregatedSignaturesTotal.Inc()
}
func (m *Metrics) IncPqSigAggregatedValid() {
	if m == nil {
		return
	}
	m.metricPqSigAggregatedValid.Inc()
}
func (m *Metrics) IncPqSigAggregatedInvalid() {
	if m == nil {
		return
	}
	m.metricPqSigAggregatedInvalid.Inc()
}
func (m *Metrics) IncPqSigAttestationsInAggregated(n int) {
	if m == nil {
		return
	}
	addCount(m.metricPqSigAttestationsInAggregated, n)
}
func (m *Metrics) IncSTFSlotsProcessed(n uint64) {
	if m == nil {
		return
	}
	addUint(m.metricSTFSlotsProcessed, n)
}
func (m *Metrics) IncSTFAttestationsProcessed(n int) {
	if m == nil {
		return
	}
	addCount(m.metricSTFAttestationsProcessed, n)
}
func (m *Metrics) IncPqSigAttestationSigsTotal() {
	if m == nil {
		return
	}
	m.metricPqSigAttestationSigsTotal.Inc()
}
func (m *Metrics) IncPqSigAttestationSigsValid() {
	if m == nil {
		return
	}
	m.metricPqSigAttestationSigsValid.Inc()
}
func (m *Metrics) IncPqSigAttestationSigsInvalid() {
	if m == nil {
		return
	}
	m.metricPqSigAttestationSigsInvalid.Inc()
}
func (m *Metrics) IncAggregationDispatchDropped() {
	if m == nil {
		return
	}
	m.metricAggregationDispatchDropped.Inc()
}
func (m *Metrics) IncAggregatorSkipped(reason string) {
	if m == nil {
		return
	}
	m.metricAggregatorSkipped.WithLabelValues(aggregatorSkipReason(reason)).Inc()
}
func (m *Metrics) IncAggregationGroupSkipped(reason string, n int) {
	if m == nil {
		return
	}
	if n <= 0 {
		return
	}
	m.metricAggregationGroupSkipped.WithLabelValues(aggregationGroupSkipReason(reason)).Add(float64(n))
}
func (m *Metrics) IncFinalization(result string) {
	if m == nil {
		return
	}
	m.metricFinalizationsTotal.WithLabelValues(labelOrUnknown(result)).Inc()
}
func (m *Metrics) IncBlockBuildingSuccess() {
	if m == nil {
		return
	}
	m.metricBlockBuildingSuccess.Inc()
}
func (m *Metrics) IncBlockBuildingFailures() {
	if m == nil {
		return
	}
	m.metricBlockBuildingFailures.Inc()
}
func (m *Metrics) IncProofOperation(operation, result string) {
	if m == nil {
		return
	}
	m.metricProofOperations.WithLabelValues(labelOrUnknown(operation), labelOrUnknown(result)).Inc()
}

func (m *Metrics) IncPeerConnection(direction, result string) {
	if m == nil {
		return
	}
	m.metricPeerConnectionEvents.WithLabelValues(labelOrUnknown(direction), labelOrUnknown(result)).Inc()
}

func (m *Metrics) IncPeerDisconnection(direction, reason string) {
	if m == nil {
		return
	}
	m.metricPeerDisconnectionEvents.WithLabelValues(labelOrUnknown(direction), labelOrUnknown(reason)).Inc()
}

func (m *Metrics) IncReqRespTimeout(protocol, direction string) {
	if m == nil {
		return
	}
	m.metricReqRespTimeout.WithLabelValues(labelOrUnknown(protocol), labelOrUnknown(direction)).Inc()
}

func (m *Metrics) IncBlockProposalAttestationBuilds() {
	if m == nil {
		return
	}
	m.metricBlockProposalAttestationBuilds.Inc()
}
