package checkpoint

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/lean/types"
)

type testSSZMarshaler interface {
	MarshalSSZ() ([]byte, error)
}

func makeTestState(slot uint64, genesisTime uint64, numValidators int) *types.State {
	validators := make([]*types.Validator, numValidators)
	for i := range numValidators {
		validators[i] = &types.Validator{
			AttestationPubkey: [types.PubkeySize]byte{byte(i + 1)},
			ProposalPubkey:    [types.PubkeySize]byte{byte(i + 11)},
			Index:             uint64(i),
		}
	}

	header := &types.BlockHeader{Slot: slot}
	justifiedSlot := uint64(0)
	if slot >= 2 {
		justifiedSlot = slot - 2
	}
	finalizedSlot := uint64(0)
	if slot >= 5 {
		finalizedSlot = slot - 5
	}

	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: genesisTime},
		Slot:                     slot,
		LatestBlockHeader:        header,
		LatestJustified:          &types.Checkpoint{Slot: justifiedSlot},
		LatestFinalized:          &types.Checkpoint{Slot: finalizedSlot},
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

func TestVerifyCheckpointStateMalformedShape(t *testing.T) {
	tests := []struct {
		name  string
		state *types.State
	}{
		{name: "nil state", state: nil},
		{name: "nil config", state: &types.State{}},
		{name: "nil header", state: &types.State{Config: &types.ChainConfig{}}},
		{
			name: "nil justified",
			state: &types.State{
				Config:            &types.ChainConfig{},
				LatestBlockHeader: &types.BlockHeader{},
			},
		},
		{
			name: "nil finalized",
			state: &types.State{
				Config:            &types.ChainConfig{},
				LatestBlockHeader: &types.BlockHeader{},
				LatestJustified:   &types.Checkpoint{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := VerifyCheckpointState(tt.state, 1000, nil); err == nil {
				t.Fatal("expected malformed state error")
			}
		})
	}
}

func TestVerifyCheckpointStateRejects(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(s *types.State, expected []*types.Validator)
		wantErr string
	}{
		{"slot zero", func(s *types.State, _ []*types.Validator) {
			s.Slot = 0
		}, "slot cannot be 0"},
		{"no validators", func(s *types.State, _ []*types.Validator) {
			s.Validators = nil
		}, "has no validators"},
		{"genesis time mismatch", func(s *types.State, _ []*types.Validator) {
			s.Config.GenesisTime = 9999
		}, "genesis time mismatch"},
		{"validator count mismatch", func(s *types.State, _ []*types.Validator) {
			s.Validators = append(s.Validators, &types.Validator{Index: 3})
		}, "validator count mismatch"},
		{"non-sequential index", func(s *types.State, _ []*types.Validator) {
			s.Validators[1].Index = 99
		}, "non-sequential index"},
		{"nil validator", func(s *types.State, _ []*types.Validator) {
			s.Validators[1] = nil
		}, "validator 1 is nil"},
		{"attestation pubkey mismatch", func(_ *types.State, expected []*types.Validator) {
			expected[1].AttestationPubkey = [types.PubkeySize]byte{0xfe}
		}, "validator 1 attestation pubkey mismatch"},
		{"proposal pubkey mismatch", func(_ *types.State, expected []*types.Validator) {
			expected[1].ProposalPubkey = [types.PubkeySize]byte{0xfe}
		}, "validator 1 proposal pubkey mismatch"},
		{"finalized exceeds state", func(s *types.State, _ []*types.Validator) {
			s.LatestFinalized.Slot = 200
		}, "finalized slot 200 exceeds state slot"},
		{"justified exceeds state", func(s *types.State, _ []*types.Validator) {
			s.LatestJustified.Slot = 200
		}, "justified slot 200 exceeds state slot"},
		{"justified precedes finalized", func(s *types.State, _ []*types.Validator) {
			s.LatestJustified.Slot, s.LatestFinalized.Slot = 90, 95
		}, "justified slot 90 precedes finalized slot 95"},
		{"justified and finalized roots differ at same slot", func(s *types.State, _ []*types.Validator) {
			s.LatestJustified.Slot, s.LatestFinalized.Slot = 50, 50
			s.LatestJustified.Root, s.LatestFinalized.Root = [32]byte{1}, [32]byte{2}
		}, "have different roots"},
		{"block header exceeds state", func(s *types.State, _ []*types.Validator) {
			s.LatestBlockHeader.Slot = 200
		}, "block header slot 200 exceeds state slot"},
		{"block header root mismatch at finalized slot", func(s *types.State, _ []*types.Validator) {
			s.LatestBlockHeader.Slot, s.LatestFinalized.Slot = 50, 50
			s.LatestFinalized.Root = [32]byte{99}
		}, "block header at finalized slot 50 has mismatched root"},
		{"block header root mismatch at justified slot", func(s *types.State, _ []*types.Validator) {
			// Above the finalized slot (95), so justified does not precede finalized.
			s.LatestBlockHeader.Slot, s.LatestJustified.Slot = 98, 98
			s.LatestJustified.Root = [32]byte{99}
		}, "block header at justified slot 98 has mismatched root"},
		{"malformed historical root", func(s *types.State, _ []*types.Validator) {
			s.HistoricalBlockHashes = [][]byte{{0x01}}
		}, "hash_tree_root"},
		// Every block on such a state fails, so a node anchored on it never
		// imports another block.
		{"justification root without votes", func(s *types.State, _ []*types.Validator) {
			s.JustificationsRoots = [][]byte{append([]byte{1}, make([]byte, 31)...)}
		}, "justifications_validators length"},
		{"justification votes without a root", func(s *types.State, _ []*types.Validator) {
			s.JustificationsValidators = types.NewBitlistSSZ(3)
		}, "justifications_validators length"},
		{"zero justification root", func(s *types.State, _ []*types.Validator) {
			s.JustificationsRoots = [][]byte{make([]byte, 32)}
			s.JustificationsValidators = types.NewBitlistSSZ(3)
		}, "zero hash"},
		{"state too far past its block to record the gap", func(s *types.State, _ []*types.Validator) {
			s.Slot = 1 << 60
		}, "slot gap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := makeTestState(100, 1000, 3)
			expected := copyValidators(state.Validators)
			tt.mutate(state, expected)
			err := VerifyCheckpointState(state, 1000, expected)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
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
		{
			in:      "http://server/lean/v0/states/finalized/extra",
			wantErr: "/lean/v0/states/finalized",
		},
		{
			in:      "http:///lean/v0/states/finalized",
			wantErr: "host",
		},
		{
			in:      "ftp://server/lean/v0/states/finalized",
			wantErr: "http or https",
		},
		{
			in:      "https://user:pass@server/lean/v0/states/finalized",
			wantErr: "user info",
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

func TestReadLimitedSSZRejectsOversizedResponse(t *testing.T) {
	body, err := readLimitedSSZ(bytes.NewReader([]byte{1, 2, 3, 4}), 3)
	if err == nil {
		t.Fatalf("expected oversized response error, got body=%v", body)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error %q does not mention size limit", err.Error())
	}
}

func TestFetchSSZSendsOctetStreamAccept(t *testing.T) {
	wantBody := []byte{0x01, 0x02, 0x03}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Fatalf("Accept=%q, want application/octet-stream", got)
		}
		writeBody(t, w, wantBody)
	}))
	defer srv.Close()

	got, err := fetchSSZ(srv.URL, time.Now().Add(checkpointFetchBudget))
	if err != nil {
		t.Fatalf("fetch ssz: %v", err)
	}
	if !bytes.Equal(got, wantBody) {
		t.Fatalf("body=%v, want %v", got, wantBody)
	}
}

// A source that is briefly unready — connection errors or non-200s while it warms
// up — must be retried rather than exiting the node on the first failure.
func TestFetchSSZRetriesUntilReady(t *testing.T) {
	defer shortCheckpointBudget(t)()
	wantBody := []byte{0x0a, 0x0b}
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeBody(t, w, wantBody)
	}))
	defer srv.Close()

	got, err := fetchSSZ(srv.URL, time.Now().Add(checkpointFetchBudget))
	if err != nil {
		t.Fatalf("fetch ssz: %v", err)
	}
	if !bytes.Equal(got, wantBody) {
		t.Fatalf("body=%v, want %v", got, wantBody)
	}
	if got := atomic.LoadInt32(&attempts); got < 3 {
		t.Fatalf("expected the unready responses to be retried, saw %d attempts", got)
	}
}

// A source that never comes ready must fail within the shared budget, not hang or
// retry forever.
func TestFetchSSZGivesUpAfterBudget(t *testing.T) {
	defer shortCheckpointBudget(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	start := time.Now()
	// A short budget keeps the test fast while still exercising several retries.
	if _, err := fetchSSZ(srv.URL, time.Now().Add(300*time.Millisecond)); err == nil {
		t.Fatal("expected an error when the source never becomes ready")
	}
	if elapsed := time.Since(start); elapsed > checkpointFetchBudget {
		t.Fatalf("fetch did not respect the budget: ran %s", elapsed)
	}
}

func copyValidators(validators []*types.Validator) []*types.Validator {
	copied := make([]*types.Validator, len(validators))
	for i, validator := range validators {
		if validator == nil {
			continue
		}
		next := *validator
		copied[i] = &next
	}
	return copied
}

func marshalSSZ(t *testing.T, value testSSZMarshaler) []byte {
	t.Helper()

	data, err := value.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal ssz: %v", err)
	}
	return data
}

func writeBody(t *testing.T, w http.ResponseWriter, body []byte) {
	t.Helper()

	if _, err := w.Write(body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func makeAnchorPair(t testing.TB) (*types.State, *types.SignedBlock) {
	t.Helper()
	state := makeTestState(100, 1000, 3)
	body := &types.BlockBody{}
	bodyRoot, err := body.HashTreeRoot()
	if err != nil {
		t.Fatalf("body hash_tree_root: %v", err)
	}
	state.LatestBlockHeader.BodyRoot = bodyRoot
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
			Body:          body,
		},
		Proof: &types.MultiMessageAggregate{},
	}
	return state, signed
}

func TestFetchCheckpointAnchor_Pairs(t *testing.T) {
	state, signed := makeAnchorPair(t)
	stateBytes := marshalSSZ(t, state)
	blockBytes := marshalSSZ(t, signed)

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		writeBody(t, w, stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		writeBody(t, w, blockBytes)
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
	signed.Block.StateRoot = [32]byte{0xff}

	stateBytes := marshalSSZ(t, state)
	blockBytes := marshalSSZ(t, signed)

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		writeBody(t, w, stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		writeBody(t, w, blockBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gotState, gotBlock, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, state.Validators)
	if err == nil {
		t.Fatal("expected pair mismatch error, got nil")
	}
	if gotState != nil || gotBlock != nil {
		t.Fatalf("expected nil results, got state=%v block=%v", gotState, gotBlock)
	}
	if !strings.Contains(err.Error(), "pair mismatch") {
		t.Errorf("error %q does not mention pair mismatch", err.Error())
	}
}

func TestVerifyAnchorPairRejectsMalformedInputs(t *testing.T) {
	if err := verifyAnchorPair(nil, &types.SignedBlock{Block: &types.Block{}}); err == nil {
		t.Fatal("expected nil state error")
	}
	if err := verifyAnchorPair(&types.State{}, nil); err == nil {
		t.Fatal("expected nil block error")
	}
	if err := verifyAnchorPair(&types.State{}, &types.SignedBlock{}); err == nil {
		t.Fatal("expected nil inner block error")
	}
}

func TestVerifyAnchorPairRejectsNilBlockBody(t *testing.T) {
	state, signed := makeAnchorPair(t)
	signed.Block.Body = nil

	if err := verifyAnchorPair(state, signed); err == nil {
		t.Fatal("expected nil body error")
	}
}

func TestVerifyAnchorPairRejectsHeaderMismatch(t *testing.T) {
	state, signed := makeAnchorPair(t)
	signed.Block.Slot++

	if err := verifyAnchorPair(state, signed); err == nil {
		t.Fatal("expected header mismatch error")
	}
}

func TestFetchCheckpointAnchor_StateNotFound(t *testing.T) {
	defer shortCheckpointBudget(t)()
	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		writeBody(t, w, []byte("unused"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gotState, gotBlock, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, nil)
	if err == nil {
		t.Fatal("expected fetch state error, got nil")
	}
	if gotState != nil || gotBlock != nil {
		t.Fatalf("expected nil results, got state=%v block=%v", gotState, gotBlock)
	}
}

func TestFetchCheckpointAnchor_BlockNotFound(t *testing.T) {
	defer shortCheckpointBudget(t)()
	state, signed := makeAnchorPair(t)
	if signed == nil {
		t.Fatal("expected signed block fixture")
	}
	stateBytes := marshalSSZ(t, state)

	mux := http.NewServeMux()
	mux.HandleFunc(StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		writeBody(t, w, stateBytes)
	})
	mux.HandleFunc(BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no finalized block yet", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	gotState, gotBlock, err := FetchCheckpointAnchor(srv.URL+StatesFinalizedPath, 1000, state.Validators)
	if err == nil {
		t.Fatal("expected fetch block error, got nil")
	}
	if gotState != nil || gotBlock != nil {
		t.Fatalf("expected nil results, got state=%v block=%v", gotState, gotBlock)
	}
}

// shortCheckpointBudget shrinks the anchor fetch budget so negative tests that
// exercise the retry path (a source that never becomes ready) finish quickly.
func shortCheckpointBudget(t *testing.T) func() {
	t.Helper()
	restoreBudget, restoreBackoff := checkpointFetchBudget, checkpointInitialBackoff
	checkpointFetchBudget = 1500 * time.Millisecond
	checkpointInitialBackoff = 50 * time.Millisecond
	return func() { checkpointFetchBudget, checkpointInitialBackoff = restoreBudget, restoreBackoff }
}
