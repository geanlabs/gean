package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/storage/db"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

func TestForkChoiceHandler(t *testing.T) {
	s := store.NewConsensusStore(db.NewInMemoryBackend())

	root := [32]byte{0x01}
	parent := [32]byte{0x02}
	s.SetHead(root)
	s.SetSafeTarget(root)
	s.SetLatestJustified(&types.Checkpoint{Root: root, Slot: 9})
	s.SetLatestFinalized(&types.Checkpoint{Root: parent, Slot: 4})
	s.InsertBlockHeader(root, &types.BlockHeader{Slot: 9, ParentRoot: parent, ProposerIndex: 3})

	state := testState(9)
	state.Validators = []*types.Validator{{Index: 0}, {Index: 1}}
	s.InsertState(root, state)

	fc := forkchoice.New(9, root, parent)

	rec := httptest.NewRecorder()
	ForkChoiceHandler(viewOf(s, fc))(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/fork_choice", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var body forkChoiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Head != "0x0100000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("head=%q, want root", body.Head)
	}
	if body.ValidatorCount != 2 {
		t.Fatalf("validator_count=%d, want 2", body.ValidatorCount)
	}
	if len(body.Nodes) != 1 {
		t.Fatalf("nodes=%d, want 1", len(body.Nodes))
	}
	if body.Nodes[0].ProposerIndex != 3 {
		t.Fatalf("proposer_index=%d, want 3", body.Nodes[0].ProposerIndex)
	}
}

// viewOf builds the fork choice view the engine would publish for s and fc.
func viewOf(s *store.ConsensusStore, fc *forkchoice.ForkChoice) func() *forkchoice.View {
	return func() *forkchoice.View {
		nodes := make([]forkchoice.ViewNode, 0)
		for _, pn := range fc.Nodes() {
			n := forkchoice.ViewNode{ProtoNode: pn}
			if header := s.GetBlockHeader(pn.Root); header != nil {
				n.ProposerIndex = header.ProposerIndex
			}
			nodes = append(nodes, n)
		}
		var validatorCount uint64
		if headState := s.GetState(s.Head()); headState != nil {
			validatorCount = headState.NumValidators()
		}
		return &forkchoice.View{
			Nodes:          nodes,
			Head:           s.Head(),
			Justified:      *s.LatestJustified(),
			Finalized:      *s.LatestFinalized(),
			SafeTarget:     s.SafeTarget(),
			ValidatorCount: validatorCount,
		}
	}
}
