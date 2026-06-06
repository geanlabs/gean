package aggregation

import (
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type Snapshot struct {
	headState    *types.State
	attSigs      map[[32]byte]*store.AttestationDataEntry
	newEntries   map[[32]byte]*store.PayloadEntry
	knownEntries map[[32]byte]*store.PayloadEntry
	targetStates map[[32]byte]*types.State
}

func SnapshotInputs(s *store.ConsensusStore) *Snapshot {
	if s.AttestationSignatures.Len() == 0 && s.NewPayloads.Len() == 0 {
		return nil
	}
	headState := s.GetState(s.Head())
	if headState == nil {
		return nil
	}

	snap := &Snapshot{
		headState:    headState,
		attSigs:      s.AttestationSignatures.Snapshot(),
		newEntries:   make(map[[32]byte]*store.PayloadEntry),
		knownEntries: make(map[[32]byte]*store.PayloadEntry),
		targetStates: make(map[[32]byte]*types.State),
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

	for dr := range dataRoots {
		attData := attestationDataForRoot(snap, dr)
		if attData == nil {
			continue
		}
		if _, ok := snap.targetStates[attData.Target.Root]; !ok {
			if state := s.GetState(attData.Target.Root); state != nil {
				snap.targetStates[attData.Target.Root] = state
			}
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
