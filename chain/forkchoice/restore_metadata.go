package forkchoice

import (
	"encoding/binary"
	"fmt"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

const (
	metaHeadKey             = "forkchoice/head"
	metaSafeTargetKey       = "forkchoice/safe_target"
	metaLatestJustifiedKey  = "forkchoice/latest_justified"
	metaLatestFinalizedKey  = "forkchoice/latest_finalized"
	metaCheckpointAnchorKey = "forkchoice/checkpoint_anchor"
)

type persistedCheckpoint struct {
	Root [32]byte
	Slot uint64
}

func cloneCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return &types.Checkpoint{}
	}
	return &types.Checkpoint{Root: cp.Root, Slot: cp.Slot}
}

func encodeRoot(root [32]byte) []byte {
	buf := make([]byte, len(root))
	copy(buf, root[:])
	return buf
}

func decodeRoot(data []byte) ([32]byte, error) {
	if len(data) != 32 {
		return [32]byte{}, fmt.Errorf("invalid root metadata length %d", len(data))
	}
	var root [32]byte
	copy(root[:], data)
	return root, nil
}

func encodeCheckpoint(cp *types.Checkpoint) ([]byte, error) {
	if cp == nil {
		return nil, fmt.Errorf("checkpoint is nil")
	}
	buf := make([]byte, 40)
	copy(buf[:32], cp.Root[:])
	binary.LittleEndian.PutUint64(buf[32:], cp.Slot)
	return buf, nil
}

func decodeCheckpoint(data []byte) (*types.Checkpoint, error) {
	if len(data) != 40 {
		return nil, fmt.Errorf("invalid checkpoint metadata length %d", len(data))
	}
	var root [32]byte
	copy(root[:], data[:32])
	return &types.Checkpoint{
		Root: root,
		Slot: binary.LittleEndian.Uint64(data[32:]),
	}, nil
}

func (c *Store) PersistRestoreMetadata() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.persistRestoreMetadataLocked()
}

func (c *Store) persistRestoreMetadataLocked() error {
	justified, err := encodeCheckpoint(c.latestJustified)
	if err != nil {
		return fmt.Errorf("encode latest justified: %w", err)
	}
	finalized, err := encodeCheckpoint(c.latestFinalized)
	if err != nil {
		return fmt.Errorf("encode latest finalized: %w", err)
	}

	if err := c.storage.PutMeta(metaHeadKey, encodeRoot(c.head)); err != nil {
		return fmt.Errorf("persist head metadata: %w", err)
	}
	if err := c.storage.PutMeta(metaSafeTargetKey, encodeRoot(c.safeTarget)); err != nil {
		return fmt.Errorf("persist safe target metadata: %w", err)
	}
	if err := c.storage.PutMeta(metaLatestJustifiedKey, justified); err != nil {
		return fmt.Errorf("persist latest justified metadata: %w", err)
	}
	if err := c.storage.PutMeta(metaLatestFinalizedKey, finalized); err != nil {
		return fmt.Errorf("persist latest finalized metadata: %w", err)
	}
	if c.hasCheckpointAnchor {
		if err := c.storage.PutMeta(metaCheckpointAnchorKey, encodeRoot(c.checkpointAnchorRoot)); err != nil {
			return fmt.Errorf("persist checkpoint anchor metadata: %w", err)
		}
	} else if err := c.storage.DeleteMeta(metaCheckpointAnchorKey); err != nil {
		return fmt.Errorf("clear checkpoint anchor metadata: %w", err)
	}

	return nil
}

func restoreFromMetadata(store storage.Store, blockSummaries map[[32]byte]blockSummary) *Store {
	headData, ok := store.GetMeta(metaHeadKey)
	if !ok {
		return nil
	}
	safeTargetData, ok := store.GetMeta(metaSafeTargetKey)
	if !ok {
		log.Warn("restore metadata missing safe target")
		return nil
	}
	justifiedData, ok := store.GetMeta(metaLatestJustifiedKey)
	if !ok {
		log.Warn("restore metadata missing latest justified")
		return nil
	}
	finalizedData, ok := store.GetMeta(metaLatestFinalizedKey)
	if !ok {
		log.Warn("restore metadata missing latest finalized")
		return nil
	}

	headRoot, err := decodeRoot(headData)
	if err != nil {
		log.Warn("restore metadata invalid head", "err", err)
		return nil
	}
	safeTargetRoot, err := decodeRoot(safeTargetData)
	if err != nil {
		log.Warn("restore metadata invalid safe target", "err", err)
		return nil
	}
	latestJustified, err := decodeCheckpoint(justifiedData)
	if err != nil {
		log.Warn("restore metadata invalid latest justified", "err", err)
		return nil
	}
	latestFinalized, err := decodeCheckpoint(finalizedData)
	if err != nil {
		log.Warn("restore metadata invalid latest finalized", "err", err)
		return nil
	}

	headState, ok := store.GetState(headRoot)
	if !ok {
		log.Warn("restore metadata head state missing", "head_root", headRoot)
		return nil
	}
	checkpointRoots := buildCheckpointRootIndex(headState, headRoot)
	if !summaryExists(headRoot, blockSummaries, checkpointRoots) {
		log.Warn("restore metadata head root missing from summaries", "head_root", headRoot)
		return nil
	}
	if !summaryExists(safeTargetRoot, blockSummaries, checkpointRoots) {
		log.Warn("restore metadata safe target root missing from summaries", "safe_target", safeTargetRoot)
		return nil
	}
	if !summaryExists(latestJustified.Root, blockSummaries, checkpointRoots) {
		log.Warn("restore metadata justified root missing from summaries", "justified_root", latestJustified.Root)
		return nil
	}
	if !summaryExists(latestFinalized.Root, blockSummaries, checkpointRoots) {
		log.Warn("restore metadata finalized root missing from summaries", "finalized_root", latestFinalized.Root)
		return nil
	}

	var checkpointAnchorRoot [32]byte
	hasCheckpointAnchor := false
	if anchorData, ok := store.GetMeta(metaCheckpointAnchorKey); ok {
		checkpointAnchorRoot, err = decodeRoot(anchorData)
		if err != nil {
			log.Warn("restore metadata invalid checkpoint anchor", "err", err)
			return nil
		}
		if _, ok := blockSummaries[checkpointAnchorRoot]; !ok {
			log.Warn("restore metadata checkpoint anchor block missing", "checkpoint_anchor", checkpointAnchorRoot)
			return nil
		}
		if _, ok := store.GetState(checkpointAnchorRoot); !ok {
			log.Warn("restore metadata checkpoint anchor state missing", "checkpoint_anchor", checkpointAnchorRoot)
			return nil
		}
		hasCheckpointAnchor = true
	}

	log.Info("chain restored from persisted metadata",
		"head_root", headRoot,
		"head_slot", headState.Slot,
		"justified_root", latestJustified.Root,
		"justified_slot", latestJustified.Slot,
		"finalized_root", latestFinalized.Root,
		"finalized_slot", latestFinalized.Slot,
		"checkpoint_anchor", checkpointAnchorRoot,
	)

	return newRestoredStore(
		store,
		headRoot,
		headState,
		blockSummaries,
		checkpointRoots,
		safeTargetRoot,
		latestJustified,
		latestFinalized,
		checkpointAnchorRoot,
		hasCheckpointAnchor,
	)
}

func newRestoredStore(
	store storage.Store,
	headRoot [32]byte,
	headState *types.State,
	blockSummaries map[[32]byte]blockSummary,
	checkpointRoots map[[32]byte]blockSummary,
	safeTargetRoot [32]byte,
	latestJustified *types.Checkpoint,
	latestFinalized *types.Checkpoint,
	checkpointAnchorRoot [32]byte,
	hasCheckpointAnchor bool,
) *Store {
	return &Store{
		time:                          headState.Slot * types.IntervalsPerSlot,
		genesisTime:                   headState.Config.GenesisTime,
		numValidators:                 uint64(len(headState.Validators)),
		head:                          headRoot,
		safeTarget:                    safeTargetRoot,
		latestJustified:               cloneCheckpoint(latestJustified),
		latestFinalized:               cloneCheckpoint(latestFinalized),
		storage:                       store,
		blockSummaries:                blockSummaries,
		checkpointRoots:               checkpointRoots,
		isAggregator:                  false,
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
		checkpointAnchorRoot:          checkpointAnchorRoot,
		hasCheckpointAnchor:           hasCheckpointAnchor,
	}
}

func summaryExists(root [32]byte, blockSummaries map[[32]byte]blockSummary, checkpointRoots map[[32]byte]blockSummary) bool {
	if root == types.ZeroHash {
		return true
	}
	if _, ok := blockSummaries[root]; ok {
		return true
	}
	if _, ok := checkpointRoots[root]; ok {
		return true
	}
	return false
}
