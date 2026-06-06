package syncer

import libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

func (sd *SyncDriver) tryReserve(peerID libp2ppeer.ID) bool {
	if sd == nil {
		return false
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if sd.inFlight == nil {
		sd.inFlight = make(map[libp2ppeer.ID]bool)
	}
	if sd.inFlight[peerID] {
		return false
	}
	sd.inFlight[peerID] = true
	return true
}

func (sd *SyncDriver) release(peerID libp2ppeer.ID) {
	if sd == nil {
		return
	}
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.inFlight, peerID)
}
