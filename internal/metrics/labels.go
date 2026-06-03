package metrics

import "strings"

const unknownLabel = "unknown"
const maxLabelRunes = 64

var syncStatusLabels = []string{"idle", "syncing", "synced", unknownLabel}

func syncStatusLabel(status string) string {
	status = labelOrUnknown(status)
	for _, label := range syncStatusLabels {
		if status == label {
			return status
		}
	}
	return unknownLabel
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
