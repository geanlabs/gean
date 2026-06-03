package node

import (
	"github.com/geanlabs/gean/internal/dutygate"
	"github.com/geanlabs/gean/internal/logger"
)

func logDutyGateEvent(event dutygate.Event) {
	switch event.Reason {
	case dutygate.ReasonNetworkStall:
		logger.Info(logger.Validator, "duty gate reopened: network stall detected. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag, event.MaxStoredSlot, event.NetworkLag)
	case dutygate.ReasonCaughtUp:
		logger.Info(logger.Validator, "duty gate reopened: local view caught up. duty=%s slot=%d head_slot=%d lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag)
	case dutygate.ReasonLocalLag:
		logger.Info(logger.Validator, "duty gate closed: local view is stale. duty=%s slot=%d head_slot=%d lag=%d max_seen_slot=%d network_lag=%d",
			event.Duty, event.Slot, event.HeadSlot, event.Lag, event.MaxStoredSlot, event.NetworkLag)
	}
}
