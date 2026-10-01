package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/internal/zkstf/zkvectors"
)

func TestReplayRoutes(t *testing.T) {
	// Slots 1, 2, 4, 5: slot 3 is empty.
	cases, err := zkvectors.Chain(zkvectors.ChainConfig{
		Name: "replay", Validators: 4, Slots: 5,
		Skip: func(slot uint64) bool { return slot == 3 },
	})
	if err != nil {
		t.Fatal(err)
	}
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	// The first block's state is left out, as finalization pruning leaves it.
	roots := make([][32]byte, len(cases))
	for i, c := range cases {
		roots[i] = storeImportedBlock(t, s, c, i > 0)
	}
	s.SetHead(roots[len(roots)-1])
	s.SetLatestFinalized(&types.Checkpoint{Root: roots[1], Slot: 2})
	pruned := roots[0]
	mux := buildAPIMux(s, nil, role.New(false))

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	for _, tc := range []struct {
		id   string
		want int // index into cases, or -1 for 404
	}{
		{"head", 3},
		{"finalized", 1},
		{"4", 2},
		{fmt.Sprintf("0x%x", roots[3]), 3},
		{"3", -1},
		{"9", -1},
		{"0x00", -1},
		{"latest", -1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			block, state := get("/lean/v0/blocks/"+tc.id+"/ssz"), get("/lean/v0/states/"+tc.id)
			if tc.want < 0 {
				if block.Code != http.StatusNotFound || state.Code != http.StatusNotFound {
					t.Fatalf("status block=%d state=%d, want 404", block.Code, state.Code)
				}
				return
			}
			var got types.SignedBlock
			if err := got.UnmarshalSSZ(block.Body.Bytes()); err != nil {
				t.Fatalf("block: status %d: %v", block.Code, err)
			}
			if root, _ := got.Block.HashTreeRoot(); root != roots[tc.want] {
				t.Fatalf("served block 0x%x, want 0x%x", root, roots[tc.want])
			}
			var post types.State
			if err := post.UnmarshalSSZ(state.Body.Bytes()); err != nil {
				t.Fatalf("state: status %d: %v", state.Code, err)
			}
			// The served post-state hashes to the block's state root, which is
			// the pre-state root of the next block.
			root, _ := post.HashTreeRoot()
			if root != cases[tc.want].Block.StateRoot {
				t.Fatalf("served state root 0x%x, want block state root 0x%x", root, cases[tc.want].Block.StateRoot)
			}
			if next := tc.want + 1; next < len(cases) {
				if pre, _ := cases[next].Pre.HashTreeRoot(); pre != root {
					t.Fatalf("served state does not chain into the next block's pre-state")
				}
			}
		})
	}

	if rec := get(fmt.Sprintf("/lean/v0/states/0x%x", pruned)); rec.Code != http.StatusNotFound {
		t.Fatalf("pruned state status=%d, want 404", rec.Code)
	}
	if rec := get(fmt.Sprintf("/lean/v0/blocks/0x%x/ssz", pruned)); rec.Code != http.StatusOK {
		t.Fatalf("block of pruned state status=%d, want 200", rec.Code)
	}
}

// storeImportedBlock writes a block, header and post-state the way block import
// does, with the post-state's cached header root filled in.
func storeImportedBlock(t *testing.T, s *store.ConsensusStore, c zkvectors.Case, withState bool) [32]byte {
	t.Helper()
	root, err := c.Block.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	bodyRoot, err := c.Block.Body.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	signed := &types.SignedBlock{Block: c.Block, Proof: &types.MultiMessageAggregate{}}
	if err := store.WriteBlockData(s, root, signed); err != nil {
		t.Fatal(err)
	}
	s.InsertBlockHeader(root, &types.BlockHeader{
		Slot: c.Block.Slot, ProposerIndex: c.Block.ProposerIndex,
		ParentRoot: c.Block.ParentRoot, StateRoot: c.Block.StateRoot, BodyRoot: bodyRoot,
	})
	if !withState {
		return root
	}
	post, err := c.Pre.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if err := statetransition.StateTransition(post, c.Block); err != nil {
		t.Fatal(err)
	}
	post.LatestBlockHeader.StateRoot = c.Block.StateRoot
	s.InsertState(root, post)
	return root
}
