package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type checkpointChanges struct {
	entries           []storage.KV
	finalizedAdvanced bool
}

func checkpointChangesFor(s *store.ConsensusStore, postState *types.State) (checkpointChanges, error) {
	if s == nil || postState == nil {
		return checkpointChanges{}, nil
	}
	currentJustified := s.LatestJustified()
	currentFinalized := s.LatestFinalized()

	var changes checkpointChanges
	if checkpointAdvanced(postState.LatestJustified, currentJustified) {
		entry, err := checkpointEntry(storage.KeyLatestJustified, postState.LatestJustified)
		if err != nil {
			return checkpointChanges{}, err
		}
		changes.entries = append(changes.entries, entry)
	}
	if checkpointAdvanced(postState.LatestFinalized, currentFinalized) {
		entry, err := checkpointEntry(storage.KeyLatestFinalized, postState.LatestFinalized)
		if err != nil {
			return checkpointChanges{}, err
		}
		changes.entries = append(changes.entries, entry)
		changes.finalizedAdvanced = true
	}
	return changes, nil
}

func checkpointAdvanced(candidate, current *types.Checkpoint) bool {
	if candidate == nil {
		return false
	}
	return current == nil || candidate.Slot > current.Slot
}

func checkpointEntry(key []byte, checkpoint *types.Checkpoint) (storage.KV, error) {
	data, err := checkpoint.MarshalSSZ()
	if err != nil {
		return storage.KV{}, fmt.Errorf("marshal checkpoint: %w", err)
	}
	return storage.KV{Key: key, Value: data}, nil
}
