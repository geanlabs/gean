package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// stateWithValidators builds a state whose registry is identifiable by index
// and whose HistoricalBlockHashes is long enough to stand in for a real one.
func stateWithValidators(t *testing.T, count int, history int) *types.State {
	t.Helper()
	st := &types.State{
		Config:            &types.ChainConfig{},
		Slot:              uint64(history),
		LatestBlockHeader: &types.BlockHeader{},
		LatestJustified:   &types.Checkpoint{},
		LatestFinalized:   &types.Checkpoint{},
	}
	st.HistoricalBlockHashes = make([][]byte, history)
	for i := range history {
		b := make([]byte, types.RootSize)
		b[0] = byte(i)
		st.HistoricalBlockHashes[i] = b
	}
	st.JustifiedSlots = make([]byte, history/8+1)
	st.JustifiedSlots[history/8] |= 1 << (history % 8)
	st.Validators = make([]*types.Validator, count)
	for i := range count {
		v := &types.Validator{Index: uint64(i)}
		v.AttestationPubkey[0] = byte(i + 1)
		st.Validators[i] = v
	}
	st.JustificationsValidators = []byte{0x01}
	return st
}

func TestValidatorKeysReadsRegistry(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	root := [32]byte{9}
	s.InsertState(root, stateWithValidators(t, 4, 32))

	keys := s.ValidatorKeys(root)
	if keys == nil {
		t.Fatal("ValidatorKeys returned nil for a stored state")
	}
	if got := keys.Len(); got != 4 {
		t.Fatalf("registry size = %d, want 4", got)
	}
	for i := range 4 {
		pubkey, ok := keys.AttestationPubkey(uint64(i))
		if !ok {
			t.Fatalf("validator %d reported out of range", i)
		}
		if pubkey[0] != byte(i+1) {
			t.Fatalf("validator %d key[0] = %d, want %d", i, pubkey[0], i+1)
		}
	}

	// Out-of-range must be reported, not panic: participant indices arrive from
	// gossip and are attacker-influenced.
	if _, ok := keys.AttestationPubkey(4); ok {
		t.Fatal("index past the registry reported in range")
	}
}

// An unknown root is how verification learns the target state is missing, so it
// has to stay distinguishable from an empty registry.
func TestValidatorKeysUnknownRootIsNil(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	if keys := s.ValidatorKeys([32]byte{7}); keys != nil {
		t.Fatalf("unknown root returned %v, want nil", keys)
	}
}

// The point of the cache is that verification stops decoding a whole state per
// attestation, so a repeat call must not touch the backend again. Without the
// cache this reads the state — 628 KB and 20,023 allocations at slot 20,000 —
// several times a second on every node.
func TestValidatorKeysCachesByRoot(t *testing.T) {
	backend := &countingBackend{Backend: storage.NewInMemoryBackend()}
	s := store.NewConsensusStore(backend)
	root := [32]byte{3}
	s.InsertState(root, stateWithValidators(t, 3, 64))

	if keys := s.ValidatorKeys(root); keys == nil {
		t.Fatal("first call returned nil")
	}
	afterFirst := backend.reads()
	if afterFirst == 0 {
		t.Fatal("first call did not read the backend")
	}

	for range 20 {
		if keys := s.ValidatorKeys(root); keys == nil || keys.Len() != 3 {
			t.Fatal("cached call returned the wrong snapshot")
		}
	}
	if got := backend.reads(); got != afterFirst {
		t.Fatalf("cached calls read the backend %d extra times, want 0", got-afterFirst)
	}
}

// The hazard this type exists to avoid. GetState hands out a fresh state on
// every call because callers such as the block processor pass the result
// straight into StateTransition, which appends to HistoricalBlockHashes and
// rewrites the registry. Had the cache sat in front of GetState, one of those
// callers would be mutating the snapshot verification reads.
func TestValidatorKeysUnaffectedByStateMutation(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	root := [32]byte{5}
	s.InsertState(root, stateWithValidators(t, 2, 16))

	// A caller in the block-processing shape: take the state, mutate it.
	mutable := s.GetState(root)
	if mutable == nil {
		t.Fatal("GetState returned nil")
	}
	mutable.Validators[0].AttestationPubkey[0] = 0xff
	mutable.Validators = mutable.Validators[:1]

	keys := s.ValidatorKeys(root)
	if keys == nil {
		t.Fatal("ValidatorKeys returned nil")
	}
	if got := keys.Len(); got != 2 {
		t.Fatalf("registry size = %d after a caller truncated its own copy, want 2", got)
	}
	pubkey, ok := keys.AttestationPubkey(0)
	if !ok {
		t.Fatal("validator 0 reported out of range")
	}
	if pubkey[0] != 1 {
		t.Fatalf("validator 0 key[0] = %d, want 1: a caller's mutation reached the snapshot", pubkey[0])
	}
}
