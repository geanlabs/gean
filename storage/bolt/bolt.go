package bolt

import (
	"fmt"
	"log"

	"github.com/geanlabs/gean/types"
	bolt "go.etcd.io/bbolt"
)

var (
	blocksBucket      = []byte("blocks")
	signedBlockBucket = []byte("signed_blocks")
	statesBucket      = []byte("states")
	metaBucket        = []byte("meta")
)

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
		for _, b := range [][]byte{blocksBucket, signedBlockBucket, statesBucket, metaBucket} {
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

func (s *Store) DeleteBlocks(roots [][32]byte) {
	s.deleteRoots(roots, blocksBucket, signedBlockBucket)
}

func (s *Store) DeleteStates(roots [][32]byte) {
	s.deleteRoots(roots, statesBucket)
}

func (s *Store) GetMeta(key string) ([]byte, bool) {
	var value []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(metaBucket).Get([]byte(key))
		if v == nil {
			return nil
		}
		value = make([]byte, len(v))
		copy(value, v)
		return nil
	})
	if err != nil {
		log.Printf("bolt: read meta %q: %v", key, err)
		return nil, false
	}
	if value == nil {
		return nil, false
	}
	return value, true
}

func (s *Store) PutMeta(key string, value []byte) error {
	buf := make([]byte, len(value))
	copy(buf, value)
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Put([]byte(key), buf)
	})
}

func (s *Store) DeleteMeta(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(metaBucket).Delete([]byte(key))
	})
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

func (s *Store) deleteRoots(roots [][32]byte, buckets ...[]byte) {
	if len(roots) == 0 || len(buckets) == 0 {
		return
	}

	const batchSize = 1000
	for start := 0; start < len(roots); start += batchSize {
		end := start + batchSize
		if end > len(roots) {
			end = len(roots)
		}
		batch := roots[start:end]
		err := s.db.Update(func(tx *bolt.Tx) error {
			for _, bucketName := range buckets {
				bucket := tx.Bucket(bucketName)
				for _, root := range batch {
					if err := bucket.Delete(root[:]); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			log.Fatalf("bolt: delete roots from %v: %v", buckets, err)
		}
	}
}
