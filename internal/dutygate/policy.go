package dutygate

type decision struct {
	allow  bool
	closed bool
	event  *Event
}

func decide(closed bool, duty string, wallSlot, headSlot, maxStoredSlot uint64) decision {
	lag := slotLag(wallSlot, headSlot)
	maxSeenSlot := maxSlot(headSlot, maxStoredSlot)
	networkLag := slotLag(wallSlot, maxSeenSlot)
	if networkLag > NetworkStallThreshold {
		return networkStallDecision(closed, duty, wallSlot, headSlot, maxSeenSlot, lag, networkLag)
	}
	if closed {
		return closedGateDecision(duty, wallSlot, headSlot, maxSeenSlot, lag, networkLag)
	}
	return openGateDecision(duty, wallSlot, headSlot, maxSeenSlot, lag, networkLag)
}

func networkStallDecision(
	closed bool,
	duty string,
	wallSlot, headSlot, maxStoredSlot, lag, networkLag uint64,
) decision {
	var event *Event
	if closed {
		event = gateEvent(ReasonNetworkStall, duty, wallSlot, headSlot, maxStoredSlot, lag, networkLag)
	}
	return decision{allow: true, closed: false, event: event}
}

func closedGateDecision(
	duty string,
	wallSlot, headSlot, maxStoredSlot, lag, networkLag uint64,
) decision {
	allow := lag <= reopenLagThreshold()
	var event *Event
	if allow {
		event = gateEvent(ReasonCaughtUp, duty, wallSlot, headSlot, maxStoredSlot, lag, networkLag)
	}
	return decision{allow: allow, closed: !allow, event: event}
}

func openGateDecision(
	duty string,
	wallSlot, headSlot, maxStoredSlot, lag, networkLag uint64,
) decision {
	allow := lag <= SyncLagThreshold
	var event *Event
	if !allow {
		event = gateEvent(ReasonLocalLag, duty, wallSlot, headSlot, maxStoredSlot, lag, networkLag)
	}
	return decision{allow: allow, closed: !allow, event: event}
}
