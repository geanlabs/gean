package node

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func TestExecutionUnresolvedPayloadDoesNotImport(t *testing.T) {
	for _, status := range []string{execution.StatusSyncing, execution.StatusAccepted, "UNKNOWN"} {
		t.Run(status, func(t *testing.T) {
			mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
				return execution.PayloadStatus{Status: status}, nil
			}}
			e := executionTestEngine(mock)
			block := &types.SignedBlock{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}}
			if e.Execution.verify(context.Background(), block) {
				t.Fatal("unresolved payload was admitted to consensus")
			}
		})
	}
}

func TestExecutionRetriesUnresolvedPayload(t *testing.T) {
	for _, terminal := range []string{execution.StatusValid, execution.StatusInvalid} {
		t.Run(terminal, func(t *testing.T) {
			var ready atomic.Bool
			var retried atomic.Bool
			mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
				if !ready.Load() {
					return execution.PayloadStatus{Status: execution.StatusSyncing}, nil
				}
				retried.Store(true)
				return execution.PayloadStatus{Status: terminal}, nil
			}}
			e := executionTestEngine(mock)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); e.runExecutionVerifier(ctx) }()
			t.Cleanup(func() { cancel(); <-done })
			block := &types.SignedBlock{Block: &types.Block{Slot: 1, Body: &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockHash: [32]byte{1}}}}}
			if !e.OnSyncBlock(ctx, block) {
				t.Fatal("delivery failed")
			}
			waitFor(t, "first execution check", func() bool { _, calls, _ := mock.Calls(); return len(calls) != 0 })
			select {
			case <-e.BlockCh:
				t.Fatal("unresolved block reached consensus")
			default:
			}
			ready.Store(true)
			waitFor(t, "execution retry", retried.Load)
			if terminal == execution.StatusValid {
				select {
				case got := <-e.BlockCh:
					if got != block {
						t.Fatal("wrong block imported")
					}
				case <-time.After(time.Second):
					t.Fatal("validated block was not released")
				}
			} else {
				select {
				case <-e.BlockCh:
					t.Fatal("invalid block reached consensus")
				case <-time.After(2 * executionRetryInterval):
				}
				_, calls, _ := mock.Calls()
				if len(calls) != 2 {
					t.Fatalf("rejected block was retained: %d calls", len(calls))
				}
			}
		})
	}
}

func TestExecutionTransportRecovery(t *testing.T) {
	var calls atomic.Int32
	mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		if calls.Add(1) == 1 {
			return execution.PayloadStatus{}, &execution.TransportError{Method: "engine_newPayloadV3", Err: errors.New("offline")}
		}
		return execution.PayloadStatus{Status: execution.StatusValid}, nil
	}}
	e := executionTestEngine(mock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	defer func() { cancel(); <-done }()
	block := &types.SignedBlock{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}}
	e.OnSyncBlock(ctx, block)
	waitFor(t, "backoff", e.Execution.unreachable)
	select {
	case <-e.BlockCh:
		t.Fatal("offline payload reached consensus")
	default:
	}
	e.Execution.mu.Lock()
	e.Execution.downUntil = time.Time{}
	e.Execution.mu.Unlock()
	select {
	case got := <-e.BlockCh:
		if got != block {
			t.Fatal("wrong recovered block")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("payload was not retried after recovery")
	}
}

func TestExecutionSubmissionRequiresValid(t *testing.T) {
	for _, status := range []string{execution.StatusValid, execution.StatusInvalid, execution.StatusInvalidBlockHash, execution.StatusSyncing, execution.StatusAccepted, "UNKNOWN", "rpc error", "transport error"} {
		t.Run(status, func(t *testing.T) {
			mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
				if status == "rpc error" {
					return execution.PayloadStatus{}, &execution.RPCError{Code: -32602, Message: "bad payload"}
				}
				if status == "transport error" {
					return execution.PayloadStatus{}, &execution.TransportError{Err: errors.New("offline")}
				}
				return execution.PayloadStatus{Status: status}, nil
			}}
			e := executionTestEngine(mock)
			payload := &types.ExecutionPayload{BlockHash: [32]byte{1}}
			want := status == execution.StatusValid
			if got := e.Execution.submit(context.Background(), payload, [32]byte{2}); got != want {
				t.Fatalf("submit=%v, want %v", got, want)
			}
			if e.Execution.payloadValidated(payload.BlockHash) != want {
				t.Fatal("incorrect cached validity")
			}
		})
	}
}

func TestExecutionRejectsRPCError(t *testing.T) {
	e := executionTestEngine(&execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		return execution.PayloadStatus{}, &execution.RPCError{Code: -32602, Message: "invalid payload"}
	}})
	block := &types.SignedBlock{Block: &types.Block{Body: &types.BlockBody{}}}
	if got := e.Execution.checkPayload(context.Background(), block); got != executionRejected {
		t.Fatalf("verdict=%v", got)
	}
}

func TestExecutionRestoredHeadRequiresConfirmation(t *testing.T) {
	status := execution.StatusSyncing
	e := executionTestEngine(&execution.Mock{OnForkchoiceUpdated: func(execution.ForkchoiceState, *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
		return execution.ForkchoiceUpdatedResult{PayloadStatus: execution.PayloadStatus{Status: status}}, nil
	}})
	head := e.Store.Head()
	e.Execution.remember(head, [32]byte{1})
	if e.Execution.headValidated() {
		t.Fatal("restored head started validated")
	}
	for _, next := range []string{execution.StatusSyncing, execution.StatusValid, execution.StatusInvalid} {
		status = next
		e.Execution.forkchoiceUpdated(context.Background(), e.Execution.forkchoiceState(), nil)
		if got := e.Execution.headValidated(); got != (next == execution.StatusValid) {
			t.Fatalf("status=%s, head validated=%v", next, got)
		}
	}
}

func TestBuildProposalChecksExecutionBeforeSigning(t *testing.T) {
	for _, status := range []string{execution.StatusInvalid, execution.StatusSyncing} {
		t.Run(status, func(t *testing.T) {
			parentHash := [32]byte{0x31}
			payload := &types.ExecutionPayload{ParentHash: parentHash, BlockHash: [32]byte{0x32}, Timestamp: statetransition.ComputeTimeAtSlot(1000, 1)}
			mock := &execution.Mock{
				OnGetPayload: func(execution.PayloadID) (*types.ExecutionPayload, error) { return payload, nil },
				OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
					return execution.PayloadStatus{Status: status}, nil
				},
			}
			e := executionTestEngine(mock)
			head, parent := proposalHeadState(t)
			head.LatestExecutionPayloadHeader.BlockHash = parentHash
			e.Store.InsertState(parent, head)
			e.Store.InsertBlockHeader(parent, head.LatestBlockHeader)
			e.Store.SetHead(parent)
			e.Execution.remember(parent, parentHash)
			e.Execution.setValidated(parentHash, true)
			e.Execution.prepared = &preparedPayload{slot: 1, parentRoot: parent, id: execution.PayloadID{1}}
			// No signing keys are installed. Execution rejection must stop the
			// proposal before reaching signing or returning a publishable result.
			if result := e.buildProposal(1, 0); result != nil {
				t.Fatal("unvalidated proposal was produced")
			}
			_, calls, _ := mock.Calls()
			if len(calls) != 1 {
				t.Fatalf("expected execution validation before signing, got %d calls", len(calls))
			}
		})
	}
}

func TestExecutionProbeRestoresConsensusHead(t *testing.T) {
	headHash, candidateHash := [32]byte{1}, [32]byte{2}
	for _, status := range []string{execution.StatusValid, execution.StatusInvalid, execution.StatusSyncing} {
		t.Run(status, func(t *testing.T) {
			mock := &execution.Mock{OnForkchoiceUpdated: func(state execution.ForkchoiceState, attrs *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
				if attrs != nil {
					t.Error("validation probe requested a build")
				}
				verdict := execution.StatusValid
				if state.HeadBlockHash == execution.Hash(candidateHash) {
					verdict = status
				}
				return execution.ForkchoiceUpdatedResult{PayloadStatus: execution.PayloadStatus{Status: verdict}}, nil
			}}
			e := executionTestEngine(mock)
			e.Execution.remember(e.Store.Head(), headHash)
			got := e.Execution.probePayload(context.Background(), candidateHash)
			want := map[string]executionVerdict{execution.StatusValid: executionValid, execution.StatusInvalid: executionRejected, execution.StatusSyncing: executionDeferred}[status]
			if got != want {
				t.Fatalf("verdict=%v, want %v", got, want)
			}
			calls, _, _ := mock.Calls()
			if len(calls) != 2 || calls[0].State.HeadBlockHash != execution.Hash(candidateHash) || calls[1].State.HeadBlockHash != execution.Hash(headHash) {
				t.Fatalf("probe/restore sequence: %+v", calls)
			}
			if calls[0].State.FinalizedBlockHash == execution.Hash(candidateHash) {
				t.Fatal("probe finalized unresolved candidate")
			}
		})
	}
}

func TestExecutionDoesNotProbeConsensusInvalidBlock(t *testing.T) {
	mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		return execution.PayloadStatus{Status: execution.StatusAccepted}, nil
	}}
	e := executionTestEngine(mock)
	block := &types.SignedBlock{Block: &types.Block{Slot: 1, ParentRoot: e.Store.Head(), Body: &types.BlockBody{}}}
	if got := e.Execution.checkPayload(context.Background(), block); got != executionRejected {
		t.Fatalf("missing proof should reject before probe, got %v", got)
	}
	if calls, _, _ := mock.Calls(); len(calls) != 0 {
		t.Fatal("consensus-invalid block changed EL fork choice")
	}
}

func TestExecutionQuarantineRequestsMissingParent(t *testing.T) {
	mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		return execution.PayloadStatus{Status: execution.StatusSyncing}, nil
	}}
	e := executionTestEngine(mock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	defer func() { cancel(); <-done }()
	parent := [32]byte{0x77}
	e.OnSyncBlock(ctx, &types.SignedBlock{Block: &types.Block{Slot: 2, ParentRoot: parent, Body: &types.BlockBody{}}})
	select {
	case got := <-e.FetchRootCh:
		if got != parent {
			t.Fatal("wrong parent requested")
		}
	case <-time.After(time.Second):
		t.Fatal("quarantine did not request missing parent")
	}
	select {
	case <-e.BlockCh:
		t.Fatal("unresolved child reached consensus")
	default:
	}
}

func TestExecutionForkchoiceNotificationsCoalesce(t *testing.T) {
	mock := &execution.Mock{}
	e := executionTestEngine(mock)
	for i := 0; i < 1000; i++ {
		e.Execution.notifyForkchoice()
	}
	if len(e.Execution.forkchoiceCh) != 1 {
		t.Fatal("forkchoice notifications did not coalesce")
	}
	e.Execution.remember(e.Store.Head(), [32]byte{9})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	defer func() { cancel(); <-done }()
	waitFor(t, "forkchoice update", func() bool { calls, _, _ := mock.Calls(); return len(calls) == 1 })
	calls, _, _ := mock.Calls()
	if calls[0].State.HeadBlockHash != (execution.Hash{9}) {
		t.Fatal("forkchoice update did not use latest head")
	}
}

func TestExecutionFullQuarantineProcessesMissingParent(t *testing.T) {
	var parentReady atomic.Bool
	mock := &execution.Mock{OnNewPayload: func(payload *types.ExecutionPayload, _ [32]byte) (execution.PayloadStatus, error) {
		if payload.BlockNumber == 1 {
			parentReady.Store(true)
			return execution.PayloadStatus{Status: execution.StatusValid}, nil
		}
		if parentReady.Load() {
			return execution.PayloadStatus{Status: execution.StatusValid}, nil
		}
		return execution.PayloadStatus{Status: execution.StatusSyncing}, nil
	}}
	e := executionTestEngine(mock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	defer func() { cancel(); <-done }()
	parent := &types.SignedBlock{Block: &types.Block{Slot: 1, Body: &types.BlockBody{ExecutionPayload: types.ExecutionPayload{BlockNumber: 1}}}}
	parentRoot, err := parent.Block.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	for slot := uint64(2); slot <= MaxPendingBlocks+1; slot++ {
		if !e.OnSyncBlock(ctx, &types.SignedBlock{Block: &types.Block{Slot: slot, ParentRoot: parentRoot, Body: &types.BlockBody{}}}) {
			t.Fatal("delivery failed")
		}
	}
	waitFor(t, "full quarantine", func() bool { _, calls, _ := mock.Calls(); return len(calls) >= MaxPendingBlocks })
	if !e.OnSyncBlock(ctx, parent) {
		t.Fatal("parent delivery failed")
	}
	select {
	case got := <-e.BlockCh:
		if got != parent {
			t.Fatal("child passed before parent validation")
		}
	case <-time.After(time.Second):
		t.Fatal("full quarantine prevented missing parent validation")
	}
	select {
	case got := <-e.BlockCh:
		if got.Block.Slot <= 1 {
			t.Fatal("expected a previously deferred child")
		}
	case <-time.After(time.Second):
		t.Fatal("children did not recover after parent validation")
	}
}

func TestExecutionQuarantineOverflowKeepsEarlierBlocks(t *testing.T) {
	mock := &execution.Mock{OnNewPayload: func(*types.ExecutionPayload, [32]byte) (execution.PayloadStatus, error) {
		return execution.PayloadStatus{Status: execution.StatusSyncing}, nil
	}}
	e := executionTestEngine(mock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runExecutionVerifier(ctx) }()
	defer func() { cancel(); <-done }()
	blockAt := func(slot uint64) *types.SignedBlock {
		return &types.SignedBlock{Block: &types.Block{Slot: slot, Body: &types.BlockBody{}}}
	}
	for slot := uint64(10); slot < MaxPendingBlocks+10; slot++ {
		if !e.OnSyncBlock(ctx, blockAt(slot)) {
			t.Fatal("delivery failed")
		}
	}
	waitFor(t, "full quarantine", func() bool { _, calls, _ := mock.Calls(); return len(calls) >= MaxPendingBlocks })
	for _, tc := range []struct{ incoming, discarded uint64 }{
		{1, MaxPendingBlocks + 9},                      // Make room for an earlier ancestor.
		{MaxPendingBlocks + 20, MaxPendingBlocks + 20}, // Retain the existing earlier blocks.
	} {
		if !e.OnSyncBlock(ctx, blockAt(tc.incoming)) {
			t.Fatal("overflow delivery failed")
		}
		want, err := blockAt(tc.discarded).Block.HashTreeRoot()
		if err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-e.FailedRootCh:
			if got != want {
				t.Fatalf("wrong block discarded for slot %d", tc.incoming)
			}
		case <-time.After(time.Second):
			t.Fatal("overflow did not release the discarded block's fetch marker")
		}
	}
	select {
	case <-e.BlockCh:
		t.Fatal("overflow admitted an unresolved block")
	default:
	}
}
