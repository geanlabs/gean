package aggregation

import (
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

type Snapshot struct {
	headState    *types.State
	headRoot     [32]byte
	slot         uint64
	attSigs      map[[32]byte]*pools.AttestationDataEntry
	newEntries   map[[32]byte]*pools.PayloadEntry
	knownEntries map[[32]byte]*pools.PayloadEntry
}

// SnapshotInputs copies the live pools. The caller supplies the head state and
// root already resolved for this dispatch, so aggregation needs no store access.
func SnapshotInputs(p *pools.Attestations, headState *types.State, headRoot [32]byte, slot uint64) *Snapshot {
	if headState == nil {
		return nil
	}
	if p.AttestationSignatures.Len() == 0 && p.NewPayloads.Len() == 0 {
		return nil
	}

	snap := &Snapshot{
		headState:    headState,
		headRoot:     headRoot,
		slot:         slot,
		attSigs:      p.AttestationSignatures.Snapshot(),
		newEntries:   make(map[[32]byte]*pools.PayloadEntry),
		knownEntries: make(map[[32]byte]*pools.PayloadEntry),
	}

	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr, entry := range p.NewPayloads.Entries() {
		dataRoots[dr] = true
		snap.newEntries[dr] = entry
	}
	knownEntries := p.KnownPayloads.Entries()
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
