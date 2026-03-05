package bolt

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/geanlabs/gean/types"
	bolt "go.etcd.io/bbolt"
)

var (
	blocksBucket      = []byte("blocks")
	signedBlockBucket = []byte("signed_blocks")
	statesBucket      = []byte("states")
	fcMetaBucket      = []byte("fc_meta")
	attestBucket      = []byte("attestations")

	fcMetaKey = []byte("meta")
)

// FCMetadataSize is the fixed binary size of persisted fork-choice metadata.
const FCMetadataSize = 168

// Store is a bbolt-backed implementation of storage.Store.
type Store struct {
	db *bolt.DB
}

// New opens (or creates) a bbolt database at path and initialises all buckets.
func New(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{blocksBucket, signedBlockBucket, statesBucket, fcMetaBucket, attestBucket} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying bbolt database.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- storage.Store implementation ---

func (s *Store) GetBlock(root [32]byte) (*types.Block, bool) {
	var blk types.Block
	found := s.get(blocksBucket, root[:], &blk)
	if !found {
		return nil, false
	}
	return &blk, true
}

func (s *Store) PutBlock(root [32]byte, block *types.Block) {
	s.put(blocksBucket, root[:], block)
}

func (s *Store) GetSignedBlock(root [32]byte) (*types.SignedBlockWithAttestation, bool) {
	var sb types.SignedBlockWithAttestation
	found := s.get(signedBlockBucket, root[:], &sb)
	if !found {
		return nil, false
	}
	return &sb, true
}

func (s *Store) PutSignedBlock(root [32]byte, sb *types.SignedBlockWithAttestation) {
	s.put(signedBlockBucket, root[:], sb)
}

func (s *Store) GetState(root [32]byte) (*types.State, bool) {
	var st types.State
	found := s.get(statesBucket, root[:], &st)
	if !found {
		return nil, false
	}
	return &st, true
}

func (s *Store) PutState(root [32]byte, state *types.State) {
	s.put(statesBucket, root[:], state)
}

func (s *Store) GetAllBlocks() map[[32]byte]*types.Block {
	result := make(map[[32]byte]*types.Block)
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(blocksBucket)
		return b.ForEach(func(k, v []byte) error {
			var blk types.Block
			if err := blk.UnmarshalSSZ(v); err != nil {
				return nil // skip corrupt entries
			}
			var key [32]byte
			copy(key[:], k)
			result[key] = &blk
			return nil
		})
	})
	return result
}

func (s *Store) GetAllStates() map[[32]byte]*types.State {
	result := make(map[[32]byte]*types.State)
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(statesBucket)
		return b.ForEach(func(k, v []byte) error {
			var st types.State
			if err := st.UnmarshalSSZ(v); err != nil {
				return nil // skip corrupt entries
			}
			var key [32]byte
			copy(key[:], k)
			result[key] = &st
			return nil
		})
	})
	return result
}

// --- SSZ helpers ---

type sszMarshaler interface {
	MarshalSSZ() ([]byte, error)
}

type sszUnmarshaler interface {
	UnmarshalSSZ([]byte) error
}

func (s *Store) put(bucket, key []byte, val sszMarshaler) {
	data, err := val.MarshalSSZ()
	if err != nil {
		log.Fatalf("bolt: marshal ssz: %v", err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).Put(key, data)
	})
	if err != nil {
		log.Fatalf("bolt: write %s: %v", bucket, err)
	}
}

func (s *Store) get(bucket, key []byte, dst sszUnmarshaler) bool {
	var found bool
	s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucket).Get(key)
		if v == nil {
			return nil
		}
		// Copy value since bolt memory is only valid inside tx.
		buf := make([]byte, len(v))
		copy(buf, v)
		if err := dst.UnmarshalSSZ(buf); err != nil {
			log.Printf("bolt: unmarshal from %s: %v", bucket, err)
			return nil
		}
		found = true
		return nil
	})
	return found
}

// --- Fork-choice metadata persistence ---

// FCMetadata holds the fork-choice state that must survive restarts.
type FCMetadata struct {
	Head          [32]byte
	SafeTarget    [32]byte
	JustifiedRoot [32]byte
	JustifiedSlot uint64
	FinalizedRoot [32]byte
	FinalizedSlot uint64
	GenesisTime   uint64
	Time          uint64
	NumValidators uint64
}

// MarshalBinary encodes FCMetadata into a 168-byte fixed-size buffer.
func (m *FCMetadata) MarshalBinary() []byte {
	buf := make([]byte, FCMetadataSize)
	off := 0
	copy(buf[off:], m.Head[:])
	off += 32
	copy(buf[off:], m.SafeTarget[:])
	off += 32
	copy(buf[off:], m.JustifiedRoot[:])
	off += 32
	binary.BigEndian.PutUint64(buf[off:], m.JustifiedSlot)
	off += 8
	copy(buf[off:], m.FinalizedRoot[:])
	off += 32
	binary.BigEndian.PutUint64(buf[off:], m.FinalizedSlot)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], m.GenesisTime)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], m.Time)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], m.NumValidators)
	return buf
}

// UnmarshalBinary decodes FCMetadata from a 168-byte buffer.
func (m *FCMetadata) UnmarshalBinary(buf []byte) error {
	if len(buf) != FCMetadataSize {
		return fmt.Errorf("fc metadata: expected %d bytes, got %d", FCMetadataSize, len(buf))
	}
	off := 0
	copy(m.Head[:], buf[off:])
	off += 32
	copy(m.SafeTarget[:], buf[off:])
	off += 32
	copy(m.JustifiedRoot[:], buf[off:])
	off += 32
	m.JustifiedSlot = binary.BigEndian.Uint64(buf[off:])
	off += 8
	copy(m.FinalizedRoot[:], buf[off:])
	off += 32
	m.FinalizedSlot = binary.BigEndian.Uint64(buf[off:])
	off += 8
	m.GenesisTime = binary.BigEndian.Uint64(buf[off:])
	off += 8
	m.Time = binary.BigEndian.Uint64(buf[off:])
	off += 8
	m.NumValidators = binary.BigEndian.Uint64(buf[off:])
	return nil
}

// PersistFCState writes fork-choice metadata and latest attestations in a single transaction.
func (s *Store) PersistFCState(meta *FCMetadata, attestations map[uint64]*types.SignedAttestation) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		// Write FC metadata.
		if err := tx.Bucket(fcMetaBucket).Put(fcMetaKey, meta.MarshalBinary()); err != nil {
			return err
		}

		// Write attestations.
		ab := tx.Bucket(attestBucket)
		for vid, sa := range attestations {
			data, err := sa.MarshalSSZ()
			if err != nil {
				return fmt.Errorf("marshal attestation %d: %w", vid, err)
			}
			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, vid)
			if err := ab.Put(key, data); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadFCMetadata reads persisted fork-choice metadata. Returns nil if the DB is fresh.
func (s *Store) LoadFCMetadata() (*FCMetadata, error) {
	var meta *FCMetadata
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(fcMetaBucket).Get(fcMetaKey)
		if v == nil {
			return nil
		}
		buf := make([]byte, len(v))
		copy(buf, v)
		meta = &FCMetadata{}
		return meta.UnmarshalBinary(buf)
	})
	return meta, err
}

// LoadAttestations reads all persisted attestations keyed by validator ID.
func (s *Store) LoadAttestations() (map[uint64]*types.SignedAttestation, error) {
	result := make(map[uint64]*types.SignedAttestation)
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(attestBucket).ForEach(func(k, v []byte) error {
			vid := binary.BigEndian.Uint64(k)
			var sa types.SignedAttestation
			buf := make([]byte, len(v))
			copy(buf, v)
			if err := sa.UnmarshalSSZ(buf); err != nil {
				return fmt.Errorf("unmarshal attestation %d: %w", vid, err)
			}
			result[vid] = &sa
			return nil
		})
	})
	return result, err
}
