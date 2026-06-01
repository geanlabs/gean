package checkpoint

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func makeTestState(slot uint64, genesisTime uint64, numValidators int) *types.State {
	validators := make([]*types.Validator, numValidators)
	for i := range numValidators {
		validators[i] = &types.Validator{
			AttestationPubkey: [types.PubkeySize]byte{byte(i + 1)},
			Index:             uint64(i),
		}
	}

	header := &types.BlockHeader{Slot: slot}

	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: genesisTime},
		Slot:                     slot,
		LatestBlockHeader:        header,
		LatestJustified:          &types.Checkpoint{Slot: slot - 2},
		LatestFinalized:          &types.Checkpoint{Slot: slot - 5},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestVerifyCheckpointStateValid(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	expectedValidators := state.Validators

	err := VerifyCheckpointState(state, 1000, expectedValidators)
	if err != nil {
		t.Fatalf("should pass: %v", err)
	}
}

func TestVerifyCheckpointStateSlotZero(t *testing.T) {
	state := makeTestState(0, 1000, 3)
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: slot is 0")
	}
}

func TestVerifyCheckpointStateNoValidators(t *testing.T) {
	state := makeTestState(100, 1000, 0)
	err := VerifyCheckpointState(state, 1000, nil)
	if err == nil {
		t.Fatal("should fail: no validators")
	}
}

func TestVerifyCheckpointStateGenesisTimeMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	err := VerifyCheckpointState(state, 9999, state.Validators)
	if err == nil {
		t.Fatal("should fail: genesis time mismatch")
	}
}

func TestVerifyCheckpointStateValidatorCountMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	twoValidators := state.Validators[:2]
	err := VerifyCheckpointState(state, 1000, twoValidators)
	if err == nil {
		t.Fatal("should fail: validator count mismatch")
	}
}

func TestVerifyCheckpointStateNonSequentialIndex(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.Validators[1].Index = 99 // break sequential
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: non-sequential index")
	}
}

func TestVerifyCheckpointStatePubkeyMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	// Different expected validators.
	expected := make([]*types.Validator, 3)
	for i := range 3 {
		expected[i] = &types.Validator{
			AttestationPubkey: [types.PubkeySize]byte{byte(i + 100)}, // different
			Index:             uint64(i),
		}
	}
	err := VerifyCheckpointState(state, 1000, expected)
	if err == nil {
		t.Fatal("should fail: pubkey mismatch")
	}
}

func TestVerifyCheckpointStateFinalizedExceedsState(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestFinalized.Slot = 200 // > state.Slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: finalized exceeds state")
	}
}

func TestVerifyCheckpointStateJustifiedPrecedesFinalized(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestJustified.Slot = 90
	state.LatestFinalized.Slot = 95 // justified < finalized
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: justified precedes finalized")
	}
}

func TestVerifyCheckpointStateJustifiedFinalizedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestJustified.Slot = 50
	state.LatestFinalized.Slot = 50
	state.LatestJustified.Root = [32]byte{1}
	state.LatestFinalized.Root = [32]byte{2} // different roots at same slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: root mismatch at same slot")
	}
}

func TestVerifyCheckpointStateBlockHeaderExceedsState(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 200 // > state.Slot
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header exceeds state")
	}
}

func TestVerifyCheckpointStateBlockHeaderFinalizedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 50
	state.LatestFinalized.Slot = 50
	state.LatestFinalized.Root = [32]byte{99} // wrong root
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header root mismatch at finalized slot")
	}
}

func TestVerifyCheckpointStateBlockHeaderJustifiedRootMismatch(t *testing.T) {
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.Slot = 90
	state.LatestJustified.Slot = 90
	state.LatestJustified.Root = [32]byte{99} // wrong root
	err := VerifyCheckpointState(state, 1000, state.Validators)
	if err == nil {
		t.Fatal("should fail: block header root mismatch at justified slot")
	}
}

func TestDeriveBlockURL(t *testing.T) {
	tests := []struct {
		in, want, wantErr string
	}{
		{
			in:   "http://server/lean/v0/states/finalized",
			want: "http://server/lean/v0/blocks/finalized",
		},
		{
			in:   "https://x.example:8080/lean/v0/states/finalized?cache=1",
			want: "https://x.example:8080/lean/v0/blocks/finalized?cache=1",
		},
		{
			in:      "http://server/some/other/path",
			wantErr: "/lean/v0/states/finalized",
		},
	}
	for _, tt := range tests {
		got, err := deriveBlockURL(tt.in)
		if tt.wantErr != "" {
			if err == nil {
				t.Errorf("deriveBlockURL(%q) want error containing %q, got nil", tt.in, tt.wantErr)
				continue
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("deriveBlockURL(%q) err=%q does not contain %q", tt.in, err.Error(), tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("deriveBlockURL(%q) unexpected err: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("deriveBlockURL(%q): got %q, want %q", tt.in, got, tt.want)
		}
	}
}

// makeAnchorPair builds a (state, signedBlock) pair where the block's
// state_root equals hash_tree_root(state), matching what a healthy
// checkpoint-server would serve. The state's LatestBlockHeader.StateRoot
// is zeroed (canonical post-state form, as gean's FinalizedStateHandler
// emits) before the inner hash so it survives roundtrip.
func makeAnchorPair(t *testing.T) (*types.State, *types.SignedBlock) {
	t.Helper()
	state := makeTestState(100, 1000, 3)
	state.LatestBlockHeader.StateRoot = types.ZeroRoot
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("state hash_tree_root: %v", err)
	}
	signed := &types.SignedBlock{
		Block: &types.Block{
			Slot:          state.Slot,
			ProposerIndex: state.LatestBlockHeader.ProposerIndex,
			ParentRoot:    state.LatestBlockHeader.ParentRoot,
			StateRoot:     stateRoot,
			Body:          &types.BlockBody{},
		},
		Signature: &types.BlockSignatures{},
	}
	return state, signed
}

func TestFetchCheckpointAnchor_Pairs(t *testing.T) {
	state, signed := makeAnchorPair(t)
	stateBytes, _ := state.MarshalSSZ()
	blockBytes, _ := signed.MarshalSSZ()

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(blockBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gotState, gotBlock, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, state.Validators)
	if err != nil {
		t.Fatalf("FetchCheckpointAnchor: %v", err)
	}
	if gotState.Slot != state.Slot {
		t.Errorf("state slot: got %d, want %d", gotState.Slot, state.Slot)
	}
	if gotBlock.Block.Slot != signed.Block.Slot {
		t.Errorf("block slot: got %d, want %d", gotBlock.Block.Slot, signed.Block.Slot)
	}
}

func TestFetchCheckpointAnchor_RejectsPairMismatch(t *testing.T) {
	state, signed := makeAnchorPair(t)
	signed.Block.StateRoot = [32]byte{0xff} // poison the pair

	stateBytes, _ := state.MarshalSSZ()
	blockBytes, _ := signed.MarshalSSZ()

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write(stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write(blockBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, _, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, state.Validators)
	if err == nil {
		t.Fatal("expected pair mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "pair mismatch") {
		t.Errorf("error %q does not mention pair mismatch", err.Error())
	}
}

func TestFetchCheckpointAnchor_StateNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("unused"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, _, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, nil)
	if err == nil {
		t.Fatal("expected fetch state error, got nil")
	}
}

func TestFetchCheckpointAnchor_BlockNotFound(t *testing.T) {
	state, _ := makeAnchorPair(t)
	stateBytes, _ := state.MarshalSSZ()

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Write(stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no finalized block yet", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, _, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, state.Validators)
	if err == nil {
		t.Fatal("expected fetch block error, got nil")
	}
}
