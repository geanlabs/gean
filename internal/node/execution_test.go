package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func executionTestEngine(mock *execution.Mock) *Engine {
	e := makeTestEngine()
	e.Execution = NewExecutionDriver(mock, e.Store, [types.AddressSize]byte{})
	return e
}

func startExecutionVerifier(t *testing.T, e *Engine) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	return ctx
}

func storeBlockWithPayloadHash(e *Engine, root [32]byte, slot uint64, parent, hash [32]byte) {
	signed := &types.SignedBlock{
		Block: &types.Block{
			Slot:       slot,
			ParentRoot: parent,
			Body:       &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockHash: hash}},
		},
		Proof: &types.MultiMessageAggregate{},
	}
	if err := e.Store.StorePendingBlock(root, signed); err != nil {
		panic(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestExecutionForkchoiceStateResolvesPayloadHashes(t *testing.T) {
	e := executionTestEngine(&execution.Mock{})
	genesisRoot := e.Store.Head()
	blockRoot := [32]byte{0xB1}
	payloadHash := [32]byte{0xE1}
	storeBlockWithPayloadHash(e, blockRoot, 1, genesisRoot, payloadHash)
	e.Store.SetHead(blockRoot)

	state := e.Execution.forkchoiceState()
	if state.HeadBlockHash != execution.Hash(payloadHash) {
		t.Fatalf("head hash: got %x", state.HeadBlockHash)
	}
	// The genesis root has no stored block in this harness, so it resolves to
	// zero rather than failing.
	if state.SafeBlockHash != (execution.Hash{}) || state.FinalizedBlockHash != (execution.Hash{}) {
		t.Fatalf("unknown roots must resolve to zero: %+v", state)
	}

	// A remembered hash wins over storage and needs no decode.
	remembered := [32]byte{0xE2}
	e.Execution.remember(blockRoot, remembered)
	if got := e.Execution.blockHash(blockRoot); got != remembered {
		t.Fatalf("remembered hash not used: %x", got)
	}
}

func TestExecutionPrepareStashesPayloadID(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	parent := e.Store.Head()
	genesisTime := e.Store.Config().GenesisTime

	mock.OnForkchoiceUpdated = func(state execution.ForkchoiceState, attrs *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
		if attrs == nil {
			t.Error("build request must carry attributes")
			return execution.ForkchoiceUpdatedResult{}, nil
		}
		if uint64(attrs.Timestamp) != statetransition.ComputeTimeAtSlot(genesisTime, 7) {
			t.Errorf("timestamp %d is not slot 7's", attrs.Timestamp)
		}
		if attrs.ParentBeaconBlockRoot != execution.Hash(parent) {
			t.Errorf("parent beacon root must be the parent root")
		}
		if attrs.Withdrawals == nil {
			t.Errorf("withdrawals must be an empty list, not null")
		}
		id := execution.PayloadID{1, 2, 3}
		return execution.ForkchoiceUpdatedResult{PayloadStatus: execution.PayloadStatus{Status: execution.StatusValid}, PayloadID: &id}, nil
	}

	e.Execution.prepare(7, parent, e.Execution.forkchoiceState(), genesisTime)
	waitFor(t, "prepared payload", func() bool {
		e.Execution.mu.Lock()
		defer e.Execution.mu.Unlock()
		return e.Execution.prepared != nil
	})
	if e.Execution.prepared.slot != 7 || e.Execution.prepared.parentRoot != parent || e.Execution.prepared.id != (execution.PayloadID{1, 2, 3}) {
		t.Fatalf("stash: %+v", e.Execution.prepared)
	}
}

func TestExecutionPrepareWithoutIDLeavesNothing(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	mock.OnForkchoiceUpdated = func(execution.ForkchoiceState, *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
		return execution.ForkchoiceUpdatedResult{PayloadStatus: execution.PayloadStatus{Status: execution.StatusSyncing}}, nil
	}
	e.Execution.prepare(7, e.Store.Head(), e.Execution.forkchoiceState(), 1000)
	waitFor(t, "forkchoice call", func() bool { fcu, _, _ := mock.Calls(); return len(fcu) == 1 })
	time.Sleep(20 * time.Millisecond)
	if e.Execution.prepared != nil {
		t.Fatal("a syncing client must not leave a payload id behind")
	}
}

func TestExecutionTakePayloadGuards(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	parent := e.Store.Head()
	built := &types.ExecutionPayload{BlockHash: [32]byte{0xAA}, Timestamp: 1}
	mock.OnGetPayload = func(id execution.PayloadID) (*types.ExecutionPayload, error) {
		if id != (execution.PayloadID{9}) {
			t.Errorf("wrong payload id %s", id)
		}
		return built, nil
	}
	ctx := context.Background()

	if payload, reason := e.Execution.takePayload(ctx, 5, parent); payload != nil || reason == "" {
		t.Fatal("nothing prepared must yield no payload")
	}

	e.Execution.prepared = &preparedPayload{slot: 5, parentRoot: parent, id: execution.PayloadID{9}}
	if payload, reason := e.Execution.takePayload(ctx, 6, parent); payload != nil || reason == "" {
		t.Fatal("a stash for another slot must not be used")
	}
	if e.Execution.prepared != nil {
		t.Fatal("a stale stash must be consumed")
	}

	e.Execution.prepared = &preparedPayload{slot: 5, parentRoot: parent, id: execution.PayloadID{9}}
	if payload, _ := e.Execution.takePayload(ctx, 5, [32]byte{0xFF}); payload != nil {
		t.Fatal("a stash built on another parent must not be used")
	}

	e.Execution.prepared = &preparedPayload{slot: 5, parentRoot: parent, id: execution.PayloadID{9}}
	payload, reason := e.Execution.takePayload(ctx, 5, parent)
	if payload == nil || reason != "" || payload.BlockHash != built.BlockHash {
		t.Fatalf("matching stash must return the built payload: %v %q", payload, reason)
	}

	e.Execution.prepared = &preparedPayload{slot: 5, parentRoot: parent, id: execution.PayloadID{9}}
	mock.OnGetPayload = func(execution.PayloadID) (*types.ExecutionPayload, error) {
		return nil, errors.New("unknown payload")
	}
	if payload, reason := e.Execution.takePayload(ctx, 5, parent); payload != nil || reason == "" {
		t.Fatal("a failed getPayload must yield no payload")
	}
}

func TestExecutionPayloadPolicy(t *testing.T) {
	block := &types.SignedBlock{Block: &types.Block{Slot: 3, ParentRoot: [32]byte{0x77}, Body: &types.BlockBody{
		ExecutionPayload: types.ExecutionPayload{BlockHash: [32]byte{1}},
	}}}
	for _, tc := range []struct {
		status  string
		err     error
		verdict executionVerdict
	}{
		{execution.StatusValid, nil, executionValid},
		{execution.StatusSyncing, nil, executionDeferred},
		{execution.StatusAccepted, nil, executionDeferred},
		{execution.StatusInvalid, nil, executionRejected},
		{execution.StatusInvalidBlockHash, nil, executionRejected},
		{"UNKNOWN", nil, executionDeferred},
		{"RPC error", errors.New("invalid payload"), executionRejected},
		{"transport error", &execution.TransportError{Err: errors.New("offline")}, executionDeferred},
	} {
		for _, operation := range []string{"import", "submit"} {
			t.Run(tc.status+"/"+operation, func(t *testing.T) {
				mock := &execution.Mock{OnNewPayload: func(payload *types.ExecutionPayload, parent [32]byte) (execution.PayloadStatus, error) {
					if payload.BlockHash != block.Block.Body.ExecutionPayload.BlockHash || parent != block.Block.ParentRoot {
						t.Error("incorrect payload or parent beacon root")
					}
					return execution.PayloadStatus{Status: tc.status}, tc.err
				}}
				e := executionTestEngine(mock)
				if operation == "import" {
					if got := e.Execution.checkPayload(t.Context(), block); got != tc.verdict {
						t.Fatalf("verdict=%v, want %v", got, tc.verdict)
					}
				} else if got := e.Execution.submit(t.Context(), &block.Block.Body.ExecutionPayload, block.Block.ParentRoot); got != (tc.verdict == executionValid) {
					t.Fatalf("submit=%v, verdict=%v", got, tc.verdict)
				}
				if e.Execution.payloadValidated(block.Block.Body.ExecutionPayload.BlockHash) != (tc.verdict == executionValid) {
					t.Fatal("incorrect cached validity")
				}
				if _, calls, _ := mock.Calls(); len(calls) != 1 {
					t.Fatalf("expected one newPayload call, got %d", len(calls))
				}
			})
		}
	}
}

func TestExecutionVerifyBacksOffWhenUnreachable(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	mock.OnNewPayload = func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		return execution.PayloadStatus{}, &execution.TransportError{Method: "engine_newPayloadV3", Err: errors.New("connection refused")}
	}
	block := &types.SignedBlock{Block: &types.Block{Slot: 3, Body: &types.BlockBody{}}}

	if e.Execution.checkPayload(t.Context(), block) != executionDeferred {
		t.Fatal("an unreachable client must not allow import")
	}
	if e.Execution.checkPayload(t.Context(), block) != executionDeferred {
		t.Fatal("backoff must not allow import")
	}
	if _, calls, _ := mock.Calls(); len(calls) != 1 {
		t.Fatalf("second verify inside the back-off window must not call the client, got %d calls", len(calls))
	}

	// Forkchoice updates honour the same window.
	if _, err := e.Execution.forkchoiceUpdated(context.Background(), execution.ForkchoiceState{}, nil); err == nil {
		t.Fatal("forkchoice update inside the back-off window must fail fast")
	}
	if fcu, _, _ := mock.Calls(); len(fcu) != 0 {
		t.Fatal("forkchoice update inside the back-off window must not reach the client")
	}
}

func TestExecutionIngressKeepsOrderAndDropsInvalid(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	mock.OnNewPayload = func(payload *types.ExecutionPayload, _ [32]byte) (execution.PayloadStatus, error) {
		if payload.BlockNumber == 2 {
			return execution.PayloadStatus{Status: execution.StatusInvalid}, nil
		}
		return execution.PayloadStatus{Status: execution.StatusValid}, nil
	}
	ctx := startExecutionVerifier(t, e)

	for slot := uint64(1); slot <= 3; slot++ {
		block := &types.SignedBlock{Block: &types.Block{Slot: slot, Body: &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockNumber: slot}}}}
		if !e.OnSyncBlock(ctx, block) {
			t.Fatalf("sync delivery of slot %d refused", slot)
		}
	}

	var got []uint64
	for len(got) < 2 {
		select {
		case block := <-e.BlockCh:
			got = append(got, block.Block.Slot)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out with %v", got)
		}
	}
	if got[0] != 1 || got[1] != 3 {
		t.Fatalf("expected slots 1 and 3 in order, got %v", got)
	}
	select {
	case block := <-e.BlockCh:
		t.Fatalf("rejected slot %d reached the dispatch loop", block.Block.Slot)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestExecutionGossipIngressDropsWhenFull(t *testing.T) {
	e := executionTestEngine(&execution.Mock{})
	block := &types.SignedBlock{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}}
	for i := 0; i < executionVerifyQueueSize; i++ {
		e.OnBlock(block)
	}
	if e.Execution.enqueue(block) {
		t.Fatal("a full verify queue must drop gossip rather than block")
	}
	select {
	case <-e.BlockCh:
		t.Fatal("gossip must not bypass verification")
	default:
	}
}
