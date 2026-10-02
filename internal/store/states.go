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

// HasState reports whether a post-state is stored for root. Block import asks
// this several times per block, of the parent among others, and a state grows
// with the chain's history: reading the value to test for it copied megabytes
// to answer yes or no. A state stored by this process has its summary in
// memory; one written before a restart is checked through ReadView.Has, which
// never copies the value out (a miss, the common case for a new block, never
// touches one at all).
func (s *ConsensusStore) HasState(root [32]byte) bool {
	if s == nil {
		return false
	}
	s.stateSummariesMu.Lock()
	_, summarized := s.stateSummaries[root]
	s.stateSummariesMu.Unlock()
	if summarized {
		return true
	}
	rv, err := s.beginRead("has state")
	if err != nil {
		return false
	}
	has, err := rv.Has(storage.TableStates, root[:])
	return err == nil && has
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
	if err := s.putOne(storage.TableStates, root[:], data, "insert state"); err != nil {
		return err
	}
	s.noteStateSummary(root, state)
	return nil
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
