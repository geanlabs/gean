package replay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/zkvectors"
)

// fakeNode serves a generated chain on the replay routes: blocks by slot and
// root, and each block's post-state by root.
func fakeNode(t *testing.T, cases []zkvectors.Case, mutateState func(*types.State)) *httptest.Server {
	t.Helper()
	blocks := map[string][]byte{}
	states := map[string][]byte{}
	for i, c := range cases {
		root, _ := c.Block.HashTreeRoot()
		enc, err := (&types.SignedBlock{Block: c.Block, Proof: &types.MultiMessageAggregate{}}).MarshalSSZ()
		if err != nil {
			t.Fatal(err)
		}
		blocks[strconv.FormatUint(c.Block.Slot, 10)] = enc
		blocks[fmt.Sprintf("0x%x", root)] = enc
		// The post-state of block i is the pre-state of block i+1; the parent
		// of the first block is served from its own pre-state.
		if i == 0 {
			pre, _ := c.Pre.Clone()
			if mutateState != nil {
				mutateState(pre)
			}
			encState, _ := pre.MarshalSSZ()
			states[fmt.Sprintf("0x%x", c.Block.ParentRoot)] = encState
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lean/v0/blocks/{id}/ssz", func(w http.ResponseWriter, r *http.Request) {
		serve(w, blocks[r.PathValue("id")])
	})
	mux.HandleFunc("GET /lean/v0/states/{id}", func(w http.ResponseWriter, r *http.Request) {
		serve(w, states[r.PathValue("id")])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func serve(w http.ResponseWriter, data []byte) {
	if data == nil {
		http.NotFound(w, nil)
		return
	}
	w.Write(data)
}

func TestReplay(t *testing.T) {
	cases, err := zkvectors.Chain(zkvectors.ChainConfig{
		Name: "replay", Validators: 4, Slots: 9,
		Skip: func(slot uint64) bool { return slot == 1 || slot == 6 },
	})
	if err != nil {
		t.Fatal(err)
	}

	ids := func(from, to uint64) []string {
		var out []string
		for slot := from; slot <= to; slot++ {
			out = append(out, strconv.FormatUint(slot, 10))
		}
		return out
	}

	for _, tc := range []struct {
		name     string
		ids      []string
		mutate   func(*types.State)
		wantErr  string
		wantSeen int
	}{
		{name: "range with empty slots", ids: ids(2, 9), wantSeen: 7},
		{name: "wrong parent state", ids: ids(2, 9),
			mutate:  func(s *types.State) { s.Config.GenesisTime++ },
			wantErr: "native transition rejects"},
		{name: "empty range", ids: ids(1, 1), wantErr: ErrNoBlocks.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := NewClient(fakeNode(t, cases, tc.mutate).URL)
			var seen []Block
			err := Run(context.Background(), node, zkstf.NativeProver{}, tc.ids, true, func(b Block) error {
				seen = append(seen, b)
				return nil
			})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(seen) != tc.wantSeen {
				t.Fatalf("replayed %d blocks, want %d", len(seen), tc.wantSeen)
			}
		})
	}

	// A single named block the node lacks is an error, not a skip.
	node := NewClient(fakeNode(t, cases, nil).URL)
	err = Run(context.Background(), node, zkstf.NativeProver{}, []string{"1"}, false, func(Block) error { return nil })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing block: err = %v, want ErrNotFound", err)
	}
}
