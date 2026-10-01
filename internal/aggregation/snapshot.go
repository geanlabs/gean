package aggregation

import (
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type Snapshot struct {
	headState    *types.State
	slot         uint64
	attSigs      map[[32]byte]*store.AttestationDataEntry
	newEntries   map[[32]byte]*store.PayloadEntry
	knownEntries map[[32]byte]*store.PayloadEntry
	// lateHeads holds the recent blocks this node first saw late in their slot.
	// See orderedGroups for why only their already-justified votes are proved.
	lateHeads map[[32]byte]bool
}

// SnapshotInputs copies the aggregation inputs out of the store. headState is
// supplied by the caller rather than fetched here: GetState decodes the whole
// state from SSZ on every call, and the dispatcher has already resolved it to
// decide whether to run at all.
func SnapshotInputs(s *store.ConsensusStore, headState *types.State, slot uint64, lateHeads map[[32]byte]bool) *Snapshot {
	if headState == nil {
		return nil
	}
	if s.AttestationSignatures.Len() == 0 && s.NewPayloads.Len() == 0 {
		return nil
	}

	snap := &Snapshot{
		headState:    headState,
		slot:         slot,
		attSigs:      s.AttestationSignatures.Snapshot(),
		newEntries:   make(map[[32]byte]*store.PayloadEntry),
		knownEntries: make(map[[32]byte]*store.PayloadEntry),
		lateHeads:    lateHeads,
	}

	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr, entry := range s.NewPayloads.Entries() {
		dataRoots[dr] = true
		snap.newEntries[dr] = entry
	}
	knownEntries := s.KnownPayloads.Entries()
	for dr := range dataRoots {
		if entry := knownEntries[dr]; entry != nil {
			snap.knownEntries[dr] = entry
		}
	}

	return snap
}

func attestationDataForRoot(snap *Snapshot, dataRoot [32]byte) *types.AttestationData {
	if e := snap.attSigs[dataRoot]; e != nil {
		return e.Data
	}
	if e := snap.newEntries[dataRoot]; e != nil {
		return e.Data
	}
	if e := snap.knownEntries[dataRoot]; e != nil {
		return e.Data
	}
	return nil
}
