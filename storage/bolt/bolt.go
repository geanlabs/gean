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
		for _, b := range [][]byte{blocksBucket, signedBlockBucket, statesBucket} {
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

func (s *Store) DeleteBlock(root [32]byte) {
	s.delete(blocksBucket, root[:])
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

func (s *Store) DeleteSignedBlock(root [32]byte) {
	s.delete(signedBlockBucket, root[:])
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

func (s *Store) DeleteState(root [32]byte) {
	s.delete(statesBucket, root[:])
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

func (s *Store) ForEachBlock(fn func(root [32]byte, block *types.Block) bool) {
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(blocksBucket)
		return b.ForEach(func(k, v []byte) error {
			var blk types.Block
			if err := blk.UnmarshalSSZ(v); err != nil {
				return nil // skip corrupt entries
			}
			var key [32]byte
			copy(key[:], k)
			if !fn(key, &blk) {
				return fmt.Errorf("stop") // break iteration
			}
			return nil
		})
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

func (s *Store) delete(bucket, key []byte) {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).Delete(key)
	})
	if err != nil {
		log.Printf("bolt: delete from %s: %v", bucket, err)
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
