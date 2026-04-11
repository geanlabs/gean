package node

import (
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

const (
	// Buffer capacities rs L87-91.
	aggregatedPayloadCap = 512
	newPayloadCap        = 64
)

// ConsensusStore holds all state required for fork choice and block processing.
//
// Note: ForkChoice does NOT live here — it lives in Engine (Phase 7),
// Engine calls ForkChoice with store data as parameters.
// with store data as parameters.
type ConsensusStore struct {
	Backend          storage.Backend
	NewPayloads      *PayloadBuffer
	KnownPayloads    *PayloadBuffer
	AttestationSignatures AttestationSignatureMap
	PubKeyCache      *xmss.PubKeyCache // cached parsed pubkey handles for aggregation
}

// NewConsensusStore creates a store backed by the given storage backend.
func NewConsensusStore(backend storage.Backend) *ConsensusStore {
	return &ConsensusStore{
		Backend:          backend,
		NewPayloads:      NewPayloadBuffer(newPayloadCap),
		KnownPayloads:    NewPayloadBuffer(aggregatedPayloadCap),
		AttestationSignatures: make(AttestationSignatureMap),
		PubKeyCache:      xmss.NewPubKeyCache(),
	}
}

// --- Metadata accessors ---

func (s *ConsensusStore) Time() uint64 {
	return s.getMetadataUint64(storage.KeyTime)
}

func (s *ConsensusStore) SetTime(t uint64) {
	s.putMetadataUint64(storage.KeyTime, t)
}

func (s *ConsensusStore) Head() [32]byte {
	return s.getMetadataRoot(storage.KeyHead)
}

func (s *ConsensusStore) SetHead(root [32]byte) {
	s.putMetadataRoot(storage.KeyHead, root)
}

func (s *ConsensusStore) SafeTarget() [32]byte {
	return s.getMetadataRoot(storage.KeySafeTarget)
}

func (s *ConsensusStore) SetSafeTarget(root [32]byte) {
	s.putMetadataRoot(storage.KeySafeTarget, root)
}

func (s *ConsensusStore) LatestJustified() *types.Checkpoint {
	return s.getMetadataCheckpoint(storage.KeyLatestJustified)
}

func (s *ConsensusStore) SetLatestJustified(cp *types.Checkpoint) {
	s.putMetadataCheckpoint(storage.KeyLatestJustified, cp)
}

func (s *ConsensusStore) LatestFinalized() *types.Checkpoint {
	return s.getMetadataCheckpoint(storage.KeyLatestFinalized)
}

func (s *ConsensusStore) SetLatestFinalized(cp *types.Checkpoint) {
	s.putMetadataCheckpoint(storage.KeyLatestFinalized, cp)
}

func (s *ConsensusStore) Config() *types.ChainConfig {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return &types.ChainConfig{}
	}
	val, err := rv.Get(storage.TableMetadata, storage.KeyConfig)
	if err != nil || val == nil {
		return &types.ChainConfig{}
	}
	cfg := &types.ChainConfig{}
	if err := cfg.UnmarshalSSZ(val); err != nil {
		return &types.ChainConfig{}
	}
	return cfg
}

func (s *ConsensusStore) SetConfig(cfg *types.ChainConfig) {
	data, _ := cfg.MarshalSSZ()
	s.putMetadata(storage.KeyConfig, data)
}

// --- Block accessors ---

func (s *ConsensusStore) GetBlockHeader(root [32]byte) *types.BlockHeader {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return nil
	}
	val, err := rv.Get(storage.TableBlockHeaders, root[:])
	if err != nil || val == nil {
		return nil
	}
	h := &types.BlockHeader{}
	if err := h.UnmarshalSSZ(val); err != nil {
		return nil
	}
	return h
}

// GetSignedBlock retrieves a full signed block from storage by root.
// Retrieves full SignedBlock SSZ from BlockSignatures table.
func (s *ConsensusStore) GetSignedBlock(root [32]byte) *types.SignedBlock {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return nil
	}

	sigBytes, _ := rv.Get(storage.TableBlockSignatures, root[:])
	if sigBytes == nil {
		return nil
	}

	full := &types.SignedBlock{}
	if err := full.UnmarshalSSZ(sigBytes); err != nil {
		return nil
	}
	if full.Block == nil {
		return nil
	}
	return full
}

// writeBlockData stores body and full signed block across split tables.
// Body in BlockBodies, full SignedBlock SSZ in BlockSignatures.
func writeBlockData(s *ConsensusStore, root [32]byte, signedBlock *types.SignedBlock) {
	wb, _ := s.Backend.BeginWrite()

	// Store body separately.
	if signedBlock.Block != nil && signedBlock.Block.Body != nil {
		bodyData, _ := signedBlock.Block.Body.MarshalSSZ()
		if len(bodyData) > 0 {
			wb.PutBatch(storage.TableBlockBodies, []storage.KV{{Key: root[:], Value: bodyData}})
		}
	}

	// Store full SignedBlock (includes block + signatures).
	fullData, _ := signedBlock.MarshalSSZ()
	wb.PutBatch(storage.TableBlockSignatures, []storage.KV{{Key: root[:], Value: fullData}})

	wb.Commit()
}

func (s *ConsensusStore) GetState(root [32]byte) *types.State {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return nil
	}
	val, err := rv.Get(storage.TableStates, root[:])
	if err != nil || val == nil {
		return nil
	}
	st := &types.State{}
	if err := st.UnmarshalSSZ(val); err != nil {
		return nil
	}
	return st
}

func (s *ConsensusStore) HasState(root [32]byte) bool {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return false
	}
	val, err := rv.Get(storage.TableStates, root[:])
	return err == nil && val != nil
}

func (s *ConsensusStore) InsertState(root [32]byte, state *types.State) {
	data, _ := state.MarshalSSZ()
	wb, _ := s.Backend.BeginWrite()
	wb.PutBatch(storage.TableStates, []storage.KV{{Key: root[:], Value: data}})
	wb.Commit()
}

// StatesCount returns the number of states currently stored.
func (s *ConsensusStore) StatesCount() int {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}
	it, err := rv.PrefixIterator(storage.TableStates, nil)
	if err != nil {
		return 0
	}
	defer it.Close()
	count := 0
	for it.Next() {
		count++
	}
	return count
}

func (s *ConsensusStore) InsertBlockHeader(root [32]byte, header *types.BlockHeader) {
	data, _ := header.MarshalSSZ()
	wb, _ := s.Backend.BeginWrite()
	wb.PutBatch(storage.TableBlockHeaders, []storage.KV{{Key: root[:], Value: data}})
	wb.Commit()
}

// HeadSlot returns the slot of the current head block.
func (s *ConsensusStore) HeadSlot() uint64 {
	h := s.GetBlockHeader(s.Head())
	if h == nil {
		return 0
	}
	return h.Slot
}

// StorePendingBlock stores block in DB without LiveChain entry (invisible to fork choice).
// Split across 3 tables: headers (for chain walk), bodies, signatures.
func (s *ConsensusStore) StorePendingBlock(root [32]byte, signedBlock *types.SignedBlock) {
	block := signedBlock.Block
	header := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
	}
	if block.Body != nil {
		bodyRoot, _ := block.Body.HashTreeRoot()
		header.BodyRoot = bodyRoot
	}
	s.InsertBlockHeader(root, header)
	writeBlockData(s, root, signedBlock)
}

// InsertLiveChainEntry adds a (slot, root) -> parent_root entry for fork choice traversal.
func (s *ConsensusStore) InsertLiveChainEntry(slot uint64, root, parentRoot [32]byte) {
	key := storage.EncodeLiveChainKey(slot, root)
	wb, _ := s.Backend.BeginWrite()
	wb.PutBatch(storage.TableLiveChain, []storage.KV{{Key: key, Value: parentRoot[:]}})
	wb.Commit()
}

// PromoteNewToKnown moves all new payloads to known.
func (s *ConsensusStore) PromoteNewToKnown() {
	entries := s.NewPayloads.Drain()
	s.KnownPayloads.PushBatch(entries)
}

// ExtractLatestKnownAttestations returns per-validator latest from known pool only.
// Used by updateHead. rs extract_latest_known_attestations (L43).
func (s *ConsensusStore) ExtractLatestKnownAttestations() map[uint64]*types.AttestationData {
	return s.KnownPayloads.ExtractLatestAttestations()
}

// ExtractLatestAllAttestations returns per-validator latest from known+new merged.
// Used by updateSafeTarget. rs extract_latest_all_attestations (L104).
func (s *ConsensusStore) ExtractLatestAllAttestations() map[uint64]*types.AttestationData {
	known := s.KnownPayloads.ExtractLatestAttestations()
	newAtts := s.NewPayloads.ExtractLatestAttestations()
	// Merge: new overwrites known if newer.
	for vid, data := range newAtts {
		existing, ok := known[vid]
		if !ok || existing.Slot < data.Slot {
			known[vid] = data
		}
	}
	return known
}

// --- Internal metadata helpers ---

func (s *ConsensusStore) getMetadataUint64(key []byte) uint64 {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || val == nil || len(val) < 8 {
		return 0
	}
	var result uint64
	for i := 0; i < 8; i++ {
		result |= uint64(val[i]) << (i * 8)
	}
	return result
}

func (s *ConsensusStore) putMetadataUint64(key []byte, val uint64) {
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(val >> (i * 8))
	}
	s.putMetadata(key, buf)
}

func (s *ConsensusStore) getMetadataRoot(key []byte) [32]byte {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return [32]byte{}
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || val == nil || len(val) < 32 {
		return [32]byte{}
	}
	var root [32]byte
	copy(root[:], val)
	return root
}

func (s *ConsensusStore) putMetadataRoot(key []byte, root [32]byte) {
	s.putMetadata(key, root[:])
}

func (s *ConsensusStore) getMetadataCheckpoint(key []byte) *types.Checkpoint {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return &types.Checkpoint{}
	}
	val, err := rv.Get(storage.TableMetadata, key)
	if err != nil || val == nil {
		return &types.Checkpoint{}
	}
	cp := &types.Checkpoint{}
	if err := cp.UnmarshalSSZ(val); err != nil {
		return &types.Checkpoint{}
	}
	return cp
}

func (s *ConsensusStore) putMetadataCheckpoint(key []byte, cp *types.Checkpoint) {
	data, _ := cp.MarshalSSZ()
	s.putMetadata(key, data)
}

func (s *ConsensusStore) putMetadata(key, value []byte) {
	wb, _ := s.Backend.BeginWrite()
	wb.PutBatch(storage.TableMetadata, []storage.KV{{Key: key, Value: value}})
	wb.Commit()
}
