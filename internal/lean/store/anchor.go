package store

import (
	"errors"
	"fmt"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/types"
)

// PutAnchor initializes the store from an anchor block's header and state in one
// batch: the config, header, state and live-chain entry, with the head, safe
// target and justified and finalized checkpoints all at the anchor. A failed
// write leaves none of them behind.
func (s *ConsensusStore) PutAnchor(root [32]byte, header *types.BlockHeader, state *types.State) error {
	if header == nil || state == nil || state.Config == nil {
		return fmt.Errorf("put anchor: header, state or config is nil")
	}
	configData, err := state.Config.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("put anchor: marshal config: %w", err)
	}
	headerData, err := header.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("put anchor: marshal header: %w", err)
	}
	stateData, err := state.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("put anchor: marshal state: %w", err)
	}
	checkpointData, err := (&types.Checkpoint{Root: root, Slot: header.Slot}).MarshalSSZ()
	if err != nil {
		return fmt.Errorf("put anchor: marshal checkpoint: %w", err)
	}

	wb, err := s.beginWrite("put anchor")
	if err != nil {
		return err
	}
	err = errors.Join(
		wb.PutBatch(TableMetadata, []storage.KV{
			{Key: keyConfig, Value: configData},
			{Key: KeyHead, Value: root[:]},
			{Key: keySafeTarget, Value: root[:]},
			{Key: keyLatestJustified, Value: checkpointData},
			{Key: keyLatestFinalized, Value: checkpointData},
		}),
		wb.PutBatch(TableBlockHeaders, []storage.KV{{Key: root[:], Value: headerData}}),
		wb.PutBatch(TableStates, []storage.KV{{Key: root[:], Value: stateData}}),
		wb.PutBatch(TableLiveChain, []storage.KV{{Key: encodeLiveChainKey(header.Slot, root), Value: header.ParentRoot[:]}}),
	)
	if err != nil {
		return fmt.Errorf("put anchor: %w", err)
	}
	if err := wb.Commit(); err != nil {
		return fmt.Errorf("put anchor: commit: %w", err)
	}
	s.noteStoredHeader(root, header)
	s.observeStoredBlockSlot(header.Slot)
	s.noteStoredState(root, state)
	s.moveCanonicalHead(root)
	return nil
}
