package store

import (
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

// PutImportedBlock persists an imported block: its header, body, signed block,
// post-state, live-chain entry and, when it advances, the justified checkpoint.
// Everything goes in one batch, so a crash mid-write leaves all of it or none.
func (s *ConsensusStore) PutImportedBlock(blockRoot [32]byte, signedBlock *types.SignedBlock, postState *types.State) error {
	if signedBlock == nil || signedBlock.Block == nil || signedBlock.Block.Body == nil {
		return fmt.Errorf("persist block: malformed signed block")
	}
	if postState == nil {
		return fmt.Errorf("persist block: post state is nil")
	}

	block := signedBlock.Block
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute body root: %w", err)
	}

	header := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
		BodyRoot:      bodyRoot,
	}
	headerData, err := header.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block header: %w", err)
	}
	stateData, err := postState.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal post state: %w", err)
	}
	fullData, err := signedBlock.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal signed block: %w", err)
	}
	bodyData, err := block.Body.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	checkpointEntries, err := s.justifiedCheckpointChange(postState)
	if err != nil {
		return err
	}

	wb, err := s.beginWrite("persist block")
	if err != nil {
		return err
	}
	if err := putImportBatch(wb, storage.TableBlockHeaders, []storage.KV{{Key: blockRoot[:], Value: headerData}}, "block header"); err != nil {
		return err
	}
	if err := putImportBatch(wb, storage.TableStates, []storage.KV{{Key: blockRoot[:], Value: stateData}}, "post state"); err != nil {
		return err
	}
	if err := putImportBatch(wb, storage.TableLiveChain, []storage.KV{{
		Key:   storage.EncodeLiveChainKey(block.Slot, blockRoot),
		Value: block.ParentRoot[:],
	}}, "live chain entry"); err != nil {
		return err
	}
	if len(bodyData) > 0 {
		if err := putImportBatch(wb, storage.TableBlockBodies, []storage.KV{{Key: blockRoot[:], Value: bodyData}}, "block body"); err != nil {
			return err
		}
	}
	if err := putImportBatch(wb, storage.TableSignedBlocks, []storage.KV{{Key: blockRoot[:], Value: fullData}}, "signed block"); err != nil {
		return err
	}
	if len(checkpointEntries) > 0 {
		if err := putImportBatch(wb, storage.TableMetadata, checkpointEntries, "checkpoints"); err != nil {
			return err
		}
	}
	if err := wb.Commit(); err != nil {
		return fmt.Errorf("persist block: commit: %w", err)
	}
	// The header is now stored, so the duty gate's stored-block high-water mark
	// has to see it, as PutBlockHeader does for pending blocks.
	s.ObserveStoredBlockSlot(block.Slot)
	return nil
}

func putImportBatch(wb storage.WriteBatch, table storage.Table, entries []storage.KV, label string) error {
	if err := wb.PutBatch(table, entries); err != nil {
		return fmt.Errorf("persist block: put %s: %w", label, err)
	}
	return nil
}

// justifiedCheckpointChange persists the post-state justified checkpoint when it
// advances. The finalized checkpoint is deliberately not advanced here: it is
// re-derived from the canonical head's chain during head selection, so a losing
// fork that finalized a higher slot cannot latch finalization above the head.
func (s *ConsensusStore) justifiedCheckpointChange(postState *types.State) ([]storage.KV, error) {
	if !checkpointAdvanced(postState.LatestJustified, s.LatestJustified()) {
		return nil, nil
	}
	entry, err := checkpointEntry(storage.KeyLatestJustified, postState.LatestJustified)
	if err != nil {
		return nil, err
	}
	return []storage.KV{entry}, nil
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
