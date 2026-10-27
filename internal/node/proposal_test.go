package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func proposalHeadState(t *testing.T) (*types.State, [32]byte) {
	t.Helper()

	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{Slot: 0},
		LatestFinalized:          &types.Checkpoint{Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	state.LatestBlockHeader.StateRoot = stateRoot

	parentRoot, err := state.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}
	return state, parentRoot
}

func TestProduceBlockWithSignaturesDoesNotPromoteNewPayloads(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 0, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 0, Root: parentRoot},
		Target: &types.Checkpoint{Slot: 0, Root: parentRoot},
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	s.NewPayloads.Push(dataRoot, data, &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices([]uint64{0}),
		Proof:        []byte{0x01},
	})

	e := &Engine{Store: s}
	block, sigs, err := e.produceBlockWithSignatures(1, 0)
	if err != nil {
		t.Fatalf("produce block: %v", err)
	}
	if block == nil {
		t.Fatal("expected produced block")
	}
	if len(sigs) != 0 {
		t.Fatalf("signature proofs=%d, want 0", len(sigs))
	}
	if s.NewPayloads.Len() != 1 || s.KnownPayloads.Len() != 0 {
		t.Fatalf("payload promotion changed buffers: new=%d known=%d", s.NewPayloads.Len(), s.KnownPayloads.Len())
	}
}

func TestPayloadsFromEntriesCopiesMapAndProofSlice(t *testing.T) {
	root := [32]byte{0x01}
	entry := &store.PayloadEntry{
		Data:   &types.AttestationData{Head: &types.Checkpoint{}, Source: &types.Checkpoint{}, Target: &types.Checkpoint{}},
		Proofs: []*types.SingleMessageAggregate{{Participants: types.BitlistFromIndices([]uint64{1})}},
	}
	original := map[[32]byte]*store.PayloadEntry{
		root: entry,
	}

	copied := payloadsFromEntries(original)
	if len(copied) != 1 {
		t.Fatalf("payloads=%d, want 1", len(copied))
	}
	copied[0].Proofs[0] = nil
	if entry.Proofs[0] == nil {
		t.Fatal("copied proof slice aliases original slice")
	}

	delete(original, root)

	if copied[0].DataRoot != root || copied[0].Data == nil {
		t.Fatal("copied payload lost entry after original map mutation")
	}
}

func TestProduceBlockWithSignaturesStaleHeadReturnsSlotError(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Slot = 1
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	_, _, err := e.produceBlockWithSignatures(1, 0)
	var stale *statetransition.StateSlotIsNewerError
	if !errors.As(err, &stale) {
		t.Fatalf("err=%v, want StateSlotIsNewerError", err)
	}
}

func TestBuildProposalSkipsStaleSlot(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Slot = 1
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	if result := e.buildProposal(proposalDuty{slot: 1}); result.signedBlock != nil || !result.retryable {
		t.Fatalf("buildProposal result=%v, want retryable failure before signing", result)
	}
}

func TestProduceBlockWithSignaturesRejectsNonProposer(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Validators = []*types.Validator{{}, {}}
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	block, sigs, err := e.produceBlockWithSignatures(1, 0)
	if err == nil {
		t.Fatal("expected non-proposer error")
	}
	if block != nil || sigs != nil {
		t.Fatalf("expected nil block and signatures, got block=%v sigs=%v", block, sigs)
	}
}

func proposalTestEngine(t *testing.T) *Engine {
	t.Helper()
	s := makeTestStore()
	state, root := proposalHeadState(t)
	s.SetHead(root)
	s.InsertState(root, state)
	s.InsertBlockHeader(root, state.LatestBlockHeader)
	return &Engine{Store: s, Keys: &xmss.KeyManager{}, ProposalCh: make(chan proposalDuty, 1), ProposalResultCh: make(chan *proposalResult, 1)}
}

func TestProposalDutyReservedThroughCompletion(t *testing.T) {
	for _, phase := range []string{"queued", "in_flight", "awaiting_acceptance", "completed"} {
		t.Run(phase, func(t *testing.T) {
			e := proposalTestEngine(t)
			e.maybePropose(1, 0)
			if phase != "queued" {
				<-e.ProposalCh
			}
			if phase == "awaiting_acceptance" {
				e.ProposalResultCh <- &proposalResult{duty: proposalDuty{slot: 1}}
			}
			if phase == "completed" {
				// A post-sign failure must also retain the reservation.
				e.acceptProposal(context.Background(), &proposalResult{duty: proposalDuty{slot: 1}})
			}
			e.maybePropose(1, 0)
			want := 0
			if phase == "queued" {
				want = 1
			}
			if len(e.ProposalCh) != want {
				t.Fatal("duplicate duty enqueued")
			}
			if phase == "queued" {
				<-e.ProposalCh
			}
			// Advancing duties must not be suppressed by the old reservation.
			e.maybePropose(2, 0)
			if len(e.ProposalCh) != 1 {
				t.Fatal("next slot suppressed")
			}
		})
	}
}

func TestProposalQueueFullDoesNotReserveDuty(t *testing.T) {
	e := proposalTestEngine(t)
	e.maybePropose(1, 0)
	e.maybePropose(2, 0)
	<-e.ProposalCh
	e.maybePropose(2, 0)
	select {
	case duty := <-e.ProposalCh:
		if duty.slot != 2 {
			t.Fatalf("slot=%d", duty.slot)
		}
	default:
		t.Fatal("queue-full attempt prevented retry")
	}
}

func TestProposalPreSignFailureReleasesOnlyMatchingDuty(t *testing.T) {
	e := proposalTestEngine(t)
	e.maybePropose(1, 0)
	first := <-e.ProposalCh
	e.acceptProposal(context.Background(), &proposalResult{duty: first, retryable: true})
	e.maybePropose(1, 0)
	if len(e.ProposalCh) != 1 {
		t.Fatal("pre-sign retry suppressed")
	}
	<-e.ProposalCh
	e.maybePropose(2, 0)
	second := <-e.ProposalCh
	e.acceptProposal(context.Background(), &proposalResult{duty: first, retryable: true})
	e.maybePropose(2, 0)
	if len(e.ProposalCh) != 0 {
		t.Fatal("old completion released newer reservation")
	}
	e.acceptProposal(context.Background(), &proposalResult{duty: second, retryable: true})
	e.maybePropose(1, 0)
	if len(e.ProposalCh) != 0 {
		t.Fatal("old slot admitted after newer duty")
	}
	e.maybePropose(2, 0)
	if len(e.ProposalCh) != 1 {
		t.Fatal("matching retry suppressed")
	}
}

func TestProposalWorkerReportsPreSignFailure(t *testing.T) {
	e := proposalTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.runProposalWorker(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("worker did not exit")
		}
	}()
	e.maybePropose(1, 0)
	select {
	case result := <-e.ProposalResultCh:
		// Empty key manager fails before signing; no real key material needed.
		if !result.retryable || result.signedBlock != nil || result.duty.slot != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
		e.acceptProposal(ctx, result)
		if e.proposalReserved {
			t.Fatal("pre-sign failure retained reservation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker failed to report completion")
	}
}

func TestProposalGateCancellationReportsRetryableFailure(t *testing.T) {
	e := proposalTestEngine(t)
	e.ProvingGate = proving.NewGate()
	if !e.ProvingGate.Acquire(context.Background(), false) {
		t.Fatal("could not occupy prover")
	}
	defer e.ProvingGate.Release(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	duty := proposalDuty{slot: 1}
	result := e.proveProposal(ctx, duty)
	if !result.retryable || result.duty != duty || result.signedBlock != nil {
		t.Fatalf("unexpected cancellation result: %+v", result)
	}
}

// The gate wait ends at slot start plus three intervals, however early the
// proposal started. Every slot of the test chain is long past that point.
func TestProposalGateWaitEndsAtSlotDeadline(t *testing.T) {
	e := proposalTestEngine(t)
	e.ProvingGate = proving.NewGate()
	if !e.ProvingGate.Acquire(context.Background(), false) {
		t.Fatal("could not occupy prover")
	}
	defer e.ProvingGate.Release(false)
	start := time.Now()
	result := e.proveProposal(context.Background(), proposalDuty{slot: 1})
	if !result.retryable || result.signedBlock != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if waited := time.Since(start); waited >= types.MillisecondsPerInterval*time.Millisecond {
		t.Fatalf("gate wait took %s after the slot's deadline had passed", waited)
	}
}

func TestProposalSigningErrorRetainsDuty(t *testing.T) {
	e := proposalTestEngine(t)
	// A closed key reaches Sign and fails without native signing or real keys.
	e.Keys = xmss.NewKeyManager(nil, map[uint64]*xmss.ValidatorKeyPair{0: {}})
	e.maybePropose(1, 0)
	duty := <-e.ProposalCh
	result := e.buildProposal(duty)
	if result.retryable || result.signedBlock != nil {
		t.Fatalf("unexpected signing failure: %+v", result)
	}
	e.acceptProposal(context.Background(), result)
	e.maybePropose(1, 0)
	if len(e.ProposalCh) != 0 {
		t.Fatal("signing error permitted another signing attempt")
	}
}

// The proposer's key has signed the block by the time it is proved, so a parent
// change mid-proof must not stop the proof: the block is still published.
func TestBlockProofContinuesWhenParentChanges(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		parentChanged, mergeFails bool
	}{
		{name: "unchanged_parent"},
		{name: "parent_changed", parentChanged: true},
		{name: "merge_error", mergeFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := proposalTestEngine(t)
			block := &types.Block{Slot: 1, ParentRoot: e.Store.Head(), Body: &types.BlockBody{}}
			root, err := block.HashTreeRoot()
			if err != nil {
				t.Fatal(err)
			}
			if tc.parentChanged {
				e.Store.SetHead([32]byte{99})
			}
			mergeCalls := 0
			proofErr := errors.New("test prover failure")
			merge := func(inputs []xmss.Type1Input, raw []xmss.RawSignature) ([]byte, error) {
				mergeCalls++
				// The proposer's signature enters the merge raw, bound to the block root
				// at the block's slot, rather than as a proof of its own.
				if len(inputs) != 0 || len(raw) != 1 || raw[0].Message != root || raw[0].Slot != 1 {
					t.Fatal("proposer signature not merged raw with its block binding")
				}
				if tc.mergeFails {
					return nil, proofErr
				}
				return []byte{8}, nil
			}
			proof, err := e.mergeBlockProofWithProver(block, nil, nil, [types.SignatureSize]byte{}, merge)
			if mergeCalls != 1 {
				t.Fatalf("merge=%d, want 1", mergeCalls)
			}
			if tc.mergeFails {
				if !errors.Is(err, proofErr) {
					t.Fatalf("lost proof error: %v", err)
				}
				return
			}
			if err != nil || len(proof) != 1 || proof[0] != 8 {
				t.Fatalf("proof=%v err=%v, want the merged proof", proof, err)
			}
		})
	}
}

// earlyProposalEngine is a proposal test engine whose head is slot 0's block
// and whose single validator proposes every slot.
// proposalTestGenesisMs is when the proposal test engine's slot 0 begins.
const proposalTestGenesisMs = 1000 * 1000

func earlyProposalEngine(t *testing.T) *Engine {
	t.Helper()
	e := proposalTestEngine(t)
	// Validator IDs come from the attestation keys.
	e.Keys = xmss.NewKeyManager(map[uint64]*xmss.ValidatorKeyPair{0: {}}, nil)
	// Fork choice keeps the head on slot 0's block through the ticks.
	head := e.Store.Head()
	e.Store.SetLatestJustified(&types.Checkpoint{Root: head})
	e.FC = forkchoice.New(0, head, [32]byte{})
	return e
}

func TestMaybeProposeEarlyWaitsForTheSlotsBlock(t *testing.T) {
	for _, tc := range []struct {
		name      string
		storeTime uint64
		wantSlot  uint64 // 0: no duty
	}{
		// The head is slot 0's block, so slot 1's proposal starts at slot 0's interval 4.
		{name: "interval_4_with_slots_block", storeTime: 4, wantSlot: 1},
		// Slot 1's block has not arrived: its successor waits for it or for interval 0.
		{name: "interval_4_without_slots_block", storeTime: types.IntervalsPerSlot + 4},
		// Interval 4's promotion has not run yet.
		{name: "interval_3", storeTime: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := earlyProposalEngine(t)
			e.Store.SetTime(tc.storeTime)
			e.maybeProposeEarly()
			select {
			case duty := <-e.ProposalCh:
				if duty.slot != tc.wantSlot {
					t.Fatalf("duty slot=%d, want %d", duty.slot, tc.wantSlot)
				}
				// The interval-0 trigger must not start a second proposal for the slot.
				e.maybePropose(duty.slot, duty.validatorID)
				if len(e.ProposalCh) != 0 {
					t.Fatal("interval-0 trigger queued a second proposal for the slot")
				}
			default:
				if tc.wantSlot != 0 {
					t.Fatalf("no duty, want slot %d", tc.wantSlot)
				}
			}
		})
	}
}

// The interval-4 tick starts the next proposal, except as the startup tick.
func TestTickAtInterval4StartsNextProposal(t *testing.T) {
	interval4 := time.UnixMilli(proposalTestGenesisMs + 4*types.MillisecondsPerInterval)

	e := earlyProposalEngine(t)
	e.tickAt(interval4)
	if len(e.ProposalCh) != 0 {
		t.Fatal("startup tick started a proposal")
	}

	e = earlyProposalEngine(t)
	e.tickAt(time.UnixMilli(proposalTestGenesisMs + 3*types.MillisecondsPerInterval))
	e.tickAt(interval4)
	select {
	case duty := <-e.ProposalCh:
		if duty.slot != 1 {
			t.Fatalf("duty slot=%d, want 1", duty.slot)
		}
	default:
		t.Fatal("interval-4 tick did not start the next proposal")
	}
}

// A block handled after the interval-4 tick starts the next proposal.
// An early duty makes the interval-0 tick skip its slot. A failure before
// signing that arrives before the slot starts is left to that tick; one that
// arrives after it is retried once, as a non-early duty.
func TestEarlyProposalFailureRetriesOnceAfterSlotStart(t *testing.T) {
	takeDuty := func(e *Engine) proposalDuty {
		t.Helper()
		select {
		case duty := <-e.ProposalCh:
			return duty
		default:
			t.Fatal("no proposal duty queued")
			return proposalDuty{}
		}
	}
	tick := func(e *Engine, interval uint64) {
		e.tickAt(time.UnixMilli(int64(proposalTestGenesisMs + interval*types.MillisecondsPerInterval)))
	}
	ctx := context.Background()

	// Before the slot starts: no immediate retry, the interval-0 tick proposes.
	e := earlyProposalEngine(t)
	tick(e, 3)
	tick(e, 4)
	early := takeDuty(e)
	e.acceptProposal(ctx, &proposalResult{duty: early, retryable: true})
	if len(e.ProposalCh) != 0 {
		t.Fatal("failure before the slot started was retried at once")
	}
	tick(e, types.IntervalsPerSlot)
	if duty := takeDuty(e); duty.slot != 1 || duty.early {
		t.Fatalf("interval-0 duty=%+v, want slot 1, not early", duty)
	}

	// After the slot starts: the interval-0 tick has already skipped the slot.
	e = earlyProposalEngine(t)
	tick(e, 3)
	tick(e, 4)
	early = takeDuty(e)
	tick(e, types.IntervalsPerSlot)
	if len(e.ProposalCh) != 0 {
		t.Fatal("interval-0 tick started a second proposal while the early one was in flight")
	}
	e.acceptProposal(ctx, &proposalResult{duty: early, retryable: true})
	retry := takeDuty(e)
	if retry.slot != 1 || retry.early {
		t.Fatalf("retry=%+v, want slot 1, not early", retry)
	}
	e.acceptProposal(ctx, &proposalResult{duty: retry, retryable: true})
	if len(e.ProposalCh) != 0 {
		t.Fatal("the retry was retried again")
	}
}

func TestDispatchBlockStartsNextProposal(t *testing.T) {
	e := earlyProposalEngine(t)
	e.BlockCh = make(chan *types.SignedBlock, 1)
	e.fetchInFlight = map[[32]byte]bool{}
	e.Store.SetTime(4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.dispatch(ctx, nil) }()
	defer func() { cancel(); <-done }()

	e.BlockCh <- blockAtSlot(0)
	select {
	case duty := <-e.ProposalCh:
		if duty.slot != 1 {
			t.Fatalf("duty slot=%d, want 1", duty.slot)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("block did not start the next proposal")
	}
}

// Only our own block being imported settles the next proposal's parent. A
// result that imports nothing must not start it: after a retryable failure that
// would requeue the failed duty until interval 0.
func TestDispatchProposalResultWithoutImportStartsNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result func(e *Engine) *proposalResult
	}{
		{name: "retryable_failure", result: func(e *Engine) *proposalResult {
			e.tickAt(time.UnixMilli(int64(proposalTestGenesisMs + 3*types.MillisecondsPerInterval)))
			e.tickAt(time.UnixMilli(int64(proposalTestGenesisMs + 4*types.MillisecondsPerInterval)))
			return &proposalResult{duty: <-e.ProposalCh, retryable: true}
		}},
		// Head is already at the block's slot, so it is discarded.
		{name: "discarded_block", result: func(e *Engine) *proposalResult {
			e.Store.SetTime(4)
			return &proposalResult{duty: proposalDuty{slot: 0}, signedBlock: blockAtSlot(0)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := earlyProposalEngine(t)
			result := tc.result(e)
			// Unbuffered: the second send returns only once the first result is handled.
			e.ProposalResultCh = make(chan *proposalResult)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); e.dispatch(ctx, nil) }()
			defer func() { cancel(); <-done }()

			e.ProposalResultCh <- result
			e.ProposalResultCh <- nil
			if len(e.ProposalCh) != 0 {
				t.Fatal("result without an import started a proposal")
			}
		})
	}
}

func TestWaitForSlotStartHoldsEarlyProposal(t *testing.T) {
	e := proposalTestEngine(t)
	// Slot 0 starts at the next whole second, so the wait is under a second.
	genesis := uint64(time.Now().Unix()) + 1
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: genesis})
	if !e.waitForSlotStart(context.Background(), 0) {
		t.Fatal("wait reported cancellation")
	}
	if now := time.Now().UnixMilli(); now < int64(genesis*1000) {
		t.Fatalf("returned %d ms before the slot started", int64(genesis*1000)-now)
	}

	e.Store.SetConfig(&types.ChainConfig{GenesisTime: genesis + 100})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e.waitForSlotStart(ctx, 0) {
		t.Fatal("cancelled wait reported the slot start")
	}
}

// A signed proposal is published even if a late block moved the head while it
// was proving. Only a head at its slot, or a parent without state, discards it.
// A block that reaches import clears its by-root fetch marker, so the marker
// shows whether acceptProposal handed the block on.
func TestAcceptProposalPublishesUnlessUnusable(t *testing.T) {
	for _, tc := range []struct {
		name        string
		setup       func(e *Engine, block *types.Block)
		wantHandled bool
	}{
		{name: "parent_still_head", setup: func(*Engine, *types.Block) {}, wantHandled: true},
		{name: "parent_replaced", setup: func(e *Engine, _ *types.Block) {
			e.Store.SetHead([32]byte{0x99})
		}, wantHandled: true},
		{name: "head_reached_slot", setup: func(e *Engine, block *types.Block) {
			later := [32]byte{0x98}
			e.Store.InsertBlockHeader(later, &types.BlockHeader{Slot: block.Slot})
			e.Store.SetHead(later)
		}},
		{name: "parent_state_gone", setup: func(_ *Engine, block *types.Block) {
			block.ParentRoot = [32]byte{0x55}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := proposalTestEngine(t)
			block := &types.Block{Slot: 1, ParentRoot: e.Store.Head(), Body: &types.BlockBody{}}
			tc.setup(e, block)
			root, err := block.HashTreeRoot()
			if err != nil {
				t.Fatal(err)
			}
			e.fetchInFlight = map[[32]byte]bool{root: true}
			e.acceptProposal(context.Background(), &proposalResult{
				blockRoot:   root,
				signedBlock: &types.SignedBlock{Block: block, Proof: &types.MultiMessageAggregate{Proof: []byte{1}}},
			})
			if handled := !e.fetchInFlight[root]; handled != tc.wantHandled {
				t.Fatalf("handed to import=%v, want %v", handled, tc.wantHandled)
			}
		})
	}
}
