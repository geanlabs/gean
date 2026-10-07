package metrics

func (m *Metrics) SetNodeInfo(name, version string) {
	if m == nil {
		return
	}
	m.metricNodeInfo.WithLabelValues(labelOrUnknown(name), labelOrUnknown(version)).Set(1)
}
func (m *Metrics) SetNodeStartTime(t float64) {
	if m == nil {
		return
	}
	setNonNegative(m.metricNodeStartTime, t)
}
func (m *Metrics) SetHeadSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricHeadSlot.Set(float64(s))
}
func (m *Metrics) SetCurrentSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricCurrentSlot.Set(float64(s))
}
func (m *Metrics) SetTickAge(seconds float64) {
	if m == nil {
		return
	}
	m.metricTickAge.Set(seconds)
}
func (m *Metrics) SetSafeTargetSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricSafeTargetSlot.Set(float64(s))
}
func (m *Metrics) SetLatestJustifiedSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricLatestJustifiedSlot.Set(float64(s))
}
func (m *Metrics) SetLatestFinalizedSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricLatestFinalizedSlot.Set(float64(s))
}
func (m *Metrics) SetJustifiedSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricJustifiedSlot.Set(float64(s))
}
func (m *Metrics) SetFinalizedSlot(s uint64) {
	if m == nil {
		return
	}
	m.metricFinalizedSlot.Set(float64(s))
}
func (m *Metrics) SetValidatorsCount(n int) {
	if m == nil {
		return
	}
	m.metricValidatorsCount.Set(countValue(n))
}

func (m *Metrics) SetIsAggregator(b bool) {
	if m == nil {
		return
	}
	m.metricIsAggregator.Set(boolValue(b))
}

func (m *Metrics) SetAttestationCommitteeCount(n uint64) {
	if m == nil {
		return
	}
	m.metricAttestationCommitteeCount.Set(float64(n))
}
func (m *Metrics) SetGossipSignatures(n int) {
	if m == nil {
		return
	}
	m.metricGossipSignatures.Set(countValue(n))
}
func (m *Metrics) SetNewAggregatedPayloads(n int) {
	if m == nil {
		return
	}
	m.metricLatestNewAggregatedPayloads.Set(countValue(n))
}
func (m *Metrics) SetKnownAggregatedPayloads(n int) {
	if m == nil {
		return
	}
	m.metricLatestKnownAggregatedPayloads.Set(countValue(n))
}
func (m *Metrics) SetPendingAttestationsTotal(n int) {
	if m == nil {
		return
	}
	m.metricPendingAttestationsTotal.Set(countValue(n))
}
func (m *Metrics) SetAttestationCommitteeSubnet(n uint64) {
	if m == nil {
		return
	}
	m.metricAttestationCommitteeSubnet.Set(float64(n))
}
func (m *Metrics) SetGossipMeshPeers(n int) {
	if m == nil {
		return
	}
	m.metricGossipMeshPeers.Set(countValue(n))
}
func (m *Metrics) SetProvingQueueDepth(operation string, n int) {
	if m == nil {
		return
	}
	m.metricProvingQueueDepth.WithLabelValues(labelOrUnknown(operation)).Set(countValue(n))
}

func (m *Metrics) SetConnectedPeers(client string, n int) {
	if m == nil {
		return
	}
	m.metricConnectedPeers.WithLabelValues(labelOrUnknown(client)).Set(countValue(n))
}

func (m *Metrics) SetSyncStatus(status string) {
	if m == nil {
		return
	}
	active := syncStatusLabel(status)
	for _, s := range syncStatusLabels {
		m.metricNodeSyncStatus.WithLabelValues(s).Set(boolValue(s == active))
	}
}

func (m *Metrics) SetAttestationAggregateCoverageValidators(section, subnet string, n int) {
	if m == nil {
		return
	}
	m.metricAttestationAggregateCoverageValidators.
		WithLabelValues(labelOrUnknown(section), labelOrUnknown(subnet)).Set(countValue(n))
}

func (m *Metrics) SetAttestationAggregateCoverageSubnets(section string, n int) {
	if m == nil {
		return
	}
	m.metricAttestationAggregateCoverageSubnets.WithLabelValues(labelOrUnknown(section)).Set(countValue(n))
}

func (m *Metrics) SetAttestationAggregateCoverageDiffValidators(direction string, n int) {
	if m == nil {
		return
	}
	m.metricAttestationAggregateCoverageDiffValidators.
		WithLabelValues(labelOrUnknown(direction)).Set(countValue(n))
}

func (m *Metrics) SetTableBytes(table string, bytes uint64) {
	if m == nil {
		return
	}
	m.metricTableBytes.WithLabelValues(labelOrUnknown(table)).Set(float64(bytes))
}
