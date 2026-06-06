package store

import (
	"fmt"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

func (s *ConsensusStore) GetState(root [32]byte) *types.State {
	rv, err := s.beginRead("get state")
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
	rv, err := s.beginRead("has state")
	if err != nil {
		return false
	}
	val, err := rv.Get(storage.TableStates, root[:])
	return err == nil && val != nil
}

func (s *ConsensusStore) InsertState(root [32]byte, state *types.State) {
	if err := s.PutState(root, state); err != nil {
		logger.Error(logger.Store, "%v", err)
	}
}

func (s *ConsensusStore) PutState(root [32]byte, state *types.State) error {
	if state == nil {
		return fmt.Errorf("insert state: state is nil")
	}
	data, err := state.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("insert state: marshal: %w", err)
	}
	return s.putOne(storage.TableStates, root[:], data, "insert state")
}

func (s *ConsensusStore) StatesCount() int {
	rv, err := s.beginRead("count states")
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
