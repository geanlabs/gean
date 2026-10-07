package syncer

func (sd *SyncDriver) tryReserve(peerID PeerID) bool {
	if sd == nil {
		return false
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if sd.inFlight == nil {
		sd.inFlight = make(map[PeerID]bool)
	}
	if sd.inFlight[peerID] {
		return false
	}
	sd.inFlight[peerID] = true
	return true
}

func (sd *SyncDriver) release(peerID PeerID) {
	if sd == nil {
		return
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.inFlight, peerID)
}
