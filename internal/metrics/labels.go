package metrics

import "strings"

const unknownLabel = "unknown"
const maxLabelRunes = 64

var syncStatusLabels = []string{"idle", "syncing", "synced", unknownLabel}

const (
	AggregatorSkipNotAggregator = "not_aggregator"
	AggregatorSkipNotSynced     = "not_synced"
	AggregatorSkipMissingState  = "missing_state"
	AggregatorSkipSpawnFailed   = "spawn_failed"
	AggregatorSkipOther         = "other"
)

var aggregatorSkipReasons = []string{
	AggregatorSkipNotAggregator,
	AggregatorSkipNotSynced,
	AggregatorSkipMissingState,
	AggregatorSkipSpawnFailed,
	AggregatorSkipOther,
}

func syncStatusLabel(status string) string {
	status = labelOrUnknown(status)
	for _, label := range syncStatusLabels {
		if status == label {
			return status
		}
	}
	return unknownLabel
}

func aggregatorSkipReason(reason string) string {
	reason = labelOrUnknown(reason)
	for _, allowed := range aggregatorSkipReasons {
		if reason == allowed {
			return reason
		}
	}
	return AggregatorSkipOther
}

func labelOrUnknown(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return unknownLabel
	}
	return truncateLabel(label, maxLabelRunes)
}

func truncateLabel(label string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for idx := range label {
		if count == maxRunes {
			return label[:idx]
		}
		count++
	}
	return label
}
