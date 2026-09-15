package node

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// Per-call deadlines for serialized FCU/probes, proposal building, and import.
const (
	executionForkchoiceTimeout  = time.Second
	executionGetPayloadTimeout  = 600 * time.Millisecond
	executionNewPayloadTimeout  = 2 * time.Second
	executionHashCacheLimit     = 4096
	executionVerifyQueueSize    = 256
	executionUnreachableBackoff = 2 * time.Second
	executionRetryInterval      = 500 * time.Millisecond
	executionRetryBatchSize     = 8
)

// Proposal skip reasons, reported in the proof-operation counter.
const (
	proposalSkipNoPayload    = "no_payload"
	proposalSkipPayloadStale = "payload_stale"
)

// ExecutionDriver owns every Engine API call the node makes, so the dispatch
// loop never waits on the execution client. It is nil on a node without one.
//
// Ownership: prepare runs on its own goroutine and hands a payload id to the
// proposal worker through the mutex. Verification and coalesced forkchoice
// notifications run on the ingress worker. The root-to-hash cache avoids
// repeatedly decoding stored blocks when assembling forkchoice updates.
type ExecutionDriver struct {
	engine       execution.Engine
	store        *store.ConsensusStore
	feeRecipient [types.AddressSize]byte

	mu        sync.Mutex
	fcuMu     sync.Mutex
	prepared  *preparedPayload
	hashes    map[[32]byte][32]byte
	validated map[[32]byte]bool
	downUntil time.Time

	verifyCh     chan *types.SignedBlock
	forkchoiceCh chan struct{}
}

type preparedPayload struct {
	slot       uint64
	parentRoot [32]byte
	id         execution.PayloadID
}

func NewExecutionDriver(engine execution.Engine, s *store.ConsensusStore, feeRecipient [types.AddressSize]byte) *ExecutionDriver {
	return &ExecutionDriver{
		engine:       engine,
		store:        s,
		feeRecipient: feeRecipient,
		hashes:       make(map[[32]byte][32]byte),
		validated:    make(map[[32]byte]bool),
		verifyCh:     make(chan *types.SignedBlock, executionVerifyQueueSize),
		forkchoiceCh: make(chan struct{}, 1),
	}
}

func (d *ExecutionDriver) setValidated(hash [32]byte, valid bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !valid {
		delete(d.validated, hash)
		return
	}
	if len(d.validated) >= executionHashCacheLimit {
		d.validated = make(map[[32]byte]bool)
	}
	d.validated[hash] = valid
}

func (d *ExecutionDriver) payloadValidated(hash [32]byte) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.validated[hash]
}

// Restored/checkpoint heads must be confirmed by this execution client before
// signing. Live heads only enter consensus after a VALID newPayload response.
func (d *ExecutionDriver) headValidated() bool {
	return d.payloadValidated(d.blockHash(d.store.Head()))
}

// remember caches imported hashes to avoid storage reads on the dispatch loop.
func (d *ExecutionDriver) remember(root, hash [32]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.hashes) >= executionHashCacheLimit {
		d.hashes = make(map[[32]byte][32]byte)
	}
	d.hashes[root] = hash
}

// blockHash falls back to storage for restored roots, or zero ("no opinion").
func (d *ExecutionDriver) blockHash(root [32]byte) [32]byte {
	if types.IsZeroRoot(root) {
		return types.ZeroRoot
	}
	d.mu.Lock()
	hash, ok := d.hashes[root]
	d.mu.Unlock()
	if ok {
		return hash
	}
	signed := d.store.GetSignedBlock(root)
	if signed == nil || signed.Block == nil || signed.Block.Body == nil {
		return types.ZeroRoot
	}
	hash = signed.Block.Body.ExecutionPayload.BlockHash
	d.remember(root, hash)
	return hash
}

// forkchoiceState is the execution client's view of head, safe, and finalized.
func (d *ExecutionDriver) forkchoiceState() execution.ForkchoiceState {
	finalized := d.blockHash(d.store.LatestFinalized().Root)
	return execution.ForkchoiceState{
		HeadBlockHash: d.blockHash(d.store.Head()),
		// Lean's SafeTarget uses a different vote view and need not be an
		// ancestor of Head. The EL requires an ancestor, so use finalized.
		SafeBlockHash:      finalized,
		FinalizedBlockHash: finalized,
	}
}

// notifyForkchoice coalesces updates. The worker reads the latest store view,
// avoiding an unbounded goroutine backlog that could replay obsolete heads.
func (d *ExecutionDriver) notifyForkchoice() {
	select {
	case d.forkchoiceCh <- struct{}{}:
	default:
	}
}

// prepare asynchronously builds on parentRoot and stashes the ID for takePayload.
func (d *ExecutionDriver) prepare(slot uint64, parentRoot [32]byte, state execution.ForkchoiceState, genesisTime uint64) {
	attrs := &execution.PayloadAttributes{
		Timestamp:             execution.Quantity(statetransition.ComputeTimeAtSlot(genesisTime, slot)),
		SuggestedFeeRecipient: d.feeRecipient,
		Withdrawals:           []execution.Withdrawal{},
		ParentBeaconBlockRoot: parentRoot,
	}
	go func() {
		d.fcuMu.Lock()
		defer d.fcuMu.Unlock()
		if d.store.Head() != parentRoot || d.store.Time()/types.IntervalsPerSlot >= slot {
			return
		}
		result, err := d.forkchoiceUpdatedLocked(context.Background(), state, attrs)
		if err != nil {
			return
		}
		if result.PayloadStatus.Status != execution.StatusValid || result.PayloadID == nil {
			logger.Warn(logger.Execution, "execution client declined to build slot=%d status=%s", slot, result.PayloadStatus.Status)
			return
		}
		d.mu.Lock()
		d.prepared = &preparedPayload{slot: slot, parentRoot: parentRoot, id: *result.PayloadID}
		d.mu.Unlock()
		logger.Info(logger.Execution, "payload build requested slot=%d payload_id=%s", slot, result.PayloadID)
	}()
}

// takePayload consumes the prepared build for slot on parentRoot. The reason
// is empty on success; otherwise it names why the proposal has no payload.
func (d *ExecutionDriver) takePayload(ctx context.Context, slot uint64, parentRoot [32]byte) (*types.ExecutionPayload, string) {
	d.mu.Lock()
	prepared := d.prepared
	d.prepared = nil
	d.mu.Unlock()

	if prepared == nil {
		metrics.IncExecutionGetPayload("not_prepared")
		return nil, "no build was prepared"
	}
	if prepared.slot != slot || prepared.parentRoot != parentRoot {
		metrics.IncExecutionGetPayload("stale")
		return nil, "prepared build is for another slot or parent"
	}

	ctx, cancel := context.WithTimeout(ctx, executionGetPayloadTimeout)
	defer cancel()
	start := time.Now()
	payload, err := d.engine.GetPayload(ctx, prepared.id)
	d.observe("engine_getPayloadV3", start, err)
	if err != nil {
		metrics.IncExecutionGetPayload("error")
		logger.Warn(logger.Execution, "getPayload failed slot=%d payload_id=%s: %v", slot, prepared.id, err)
		return nil, "getPayload failed"
	}
	if err := payload.ValidateExecutionFeatures(); err != nil {
		logger.Warn(logger.Execution, "unsupported payload slot=%d: %v", slot, err)
		return nil, "unsupported execution payload"
	}
	metrics.IncExecutionGetPayload("success")
	return payload, ""
}

// submit requires VALID before signing; getPayload alone cannot establish validity.
func (d *ExecutionDriver) submit(ctx context.Context, payload *types.ExecutionPayload, parentRoot [32]byte) bool {
	status, err := d.newPayload(ctx, payload, parentRoot)
	if err != nil {
		logger.Warn(logger.Execution, "own payload not accepted by the execution client: %v", err)
		return false
	}
	metrics.IncExecutionNewPayload(strings.ToLower(status.Status))
	if status.Status != execution.StatusValid {
		logger.Warn(logger.Execution, "own payload status=%s", status.Status)
		return false
	}
	d.setValidated(payload.BlockHash, true)
	return true
}

// enqueue offers a gossiped block for verification without blocking; a full
// queue drops it, as the gossip path always has.
func (d *ExecutionDriver) enqueue(block *types.SignedBlock) bool {
	select {
	case d.verifyCh <- block:
		return true
	default:
		return false
	}
}

// enqueueSync offers a requested block for verification, waiting for room so
// a block this node asked for is never dropped.
func (d *ExecutionDriver) enqueueSync(ctx context.Context, block *types.SignedBlock) bool {
	select {
	case d.verifyCh <- block:
		return true
	case <-ctx.Done():
		return false
	}
}

type executionVerdict uint8

const (
	executionRejected executionVerdict = iota
	executionDeferred
	executionValid
)

func (d *ExecutionDriver) checkPayload(ctx context.Context, block *types.SignedBlock) executionVerdict {
	if block == nil || block.Block == nil || block.Block.Body == nil {
		return executionRejected
	}
	payload := &block.Block.Body.ExecutionPayload
	parentRoot := block.Block.ParentRoot

	status, err := d.newPayload(ctx, payload, parentRoot)
	if err != nil {
		if execution.IsTransport(err) {
			metrics.IncExecutionNewPayload(metrics.ExecutionResultUnreachable)
			return executionDeferred
		}
		logger.Warn(logger.Execution, "newPayload rejected slot=%d: %v", block.Block.Slot, err)
		return executionRejected
	}

	metrics.IncExecutionNewPayload(strings.ToLower(status.Status))
	switch status.Status {
	case execution.StatusValid:
		d.setValidated(payload.BlockHash, true)
		return executionValid
	case execution.StatusSyncing, execution.StatusAccepted:
		return d.resolveDeferred(ctx, block)
	case execution.StatusInvalid, execution.StatusInvalidBlockHash:
		metrics.IncExecutionBlocksRejected()
		reason := ""
		if status.ValidationError != nil {
			reason = *status.ValidationError
		}
		logger.Warn(logger.Execution, "execution client rejected block slot=%d status=%s %s", block.Block.Slot, status.Status, reason)
		d.setValidated(payload.BlockHash, false)
		return executionRejected
	default:
		logger.Warn(logger.Execution, "unknown newPayload status %q slot=%d, deferring", status.Status, block.Block.Slot)
		return executionDeferred
	}
}

// Some engines defer side-branch execution until forkchoiceUpdated selects the
// candidate. Check CL validity first, then probe without finalizing it and restore
// the actual CL head. No unresolved candidate enters the consensus store.
func (d *ExecutionDriver) resolveDeferred(ctx context.Context, block *types.SignedBlock) executionVerdict {
	if !d.store.HasState(block.Block.ParentRoot) {
		return executionDeferred
	}
	if err := blockprocessor.ValidateBlock(d.store, block); err != nil {
		logger.Warn(logger.Execution, "rejecting consensus-invalid execution candidate slot=%d: %v", block.Block.Slot, err)
		return executionRejected
	}
	return d.probePayload(ctx, block.Block.Body.ExecutionPayload.BlockHash)
}

func (d *ExecutionDriver) probePayload(ctx context.Context, hash [32]byte) executionVerdict {
	d.fcuMu.Lock()
	defer d.fcuMu.Unlock()
	state := d.forkchoiceState()
	state.HeadBlockHash = hash
	result, err := d.forkchoiceUpdatedLocked(ctx, state, nil)
	// Resolve the restore state after the probe: the dispatch loop may have
	// advanced meanwhile. Serialize both calls with normal FCU/build requests.
	_, _ = d.forkchoiceUpdatedLocked(ctx, d.forkchoiceState(), nil)
	if err != nil {
		return executionDeferred
	}
	switch result.PayloadStatus.Status {
	case execution.StatusValid:
		return executionValid
	case execution.StatusInvalid, execution.StatusInvalidBlockHash:
		return executionRejected
	default:
		return executionDeferred
	}
}

func (d *ExecutionDriver) newPayload(ctx context.Context, payload *types.ExecutionPayload, parentRoot [32]byte) (execution.PayloadStatus, error) {
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return execution.PayloadStatus{}, err
	}
	if d.unreachable() {
		return execution.PayloadStatus{}, &execution.TransportError{Method: "engine_newPayloadV3", Err: errUnreachableBackoff}
	}
	ctx, cancel := context.WithTimeout(ctx, executionNewPayloadTimeout)
	defer cancel()
	start := time.Now()
	status, err := d.engine.NewPayload(ctx, payload, parentRoot)
	d.observe("engine_newPayloadV3", start, err)
	return status, err
}

func (d *ExecutionDriver) forkchoiceUpdated(ctx context.Context, state execution.ForkchoiceState, attrs *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
	d.fcuMu.Lock()
	defer d.fcuMu.Unlock()
	return d.forkchoiceUpdatedLocked(ctx, state, attrs)
}

func (d *ExecutionDriver) forkchoiceUpdatedLocked(ctx context.Context, state execution.ForkchoiceState, attrs *execution.PayloadAttributes) (execution.ForkchoiceUpdatedResult, error) {
	if d.unreachable() {
		return execution.ForkchoiceUpdatedResult{}, &execution.TransportError{Method: "engine_forkchoiceUpdatedV3", Err: errUnreachableBackoff}
	}
	ctx, cancel := context.WithTimeout(ctx, executionForkchoiceTimeout)
	defer cancel()
	start := time.Now()
	result, err := d.engine.ForkchoiceUpdated(ctx, state, attrs)
	d.observe("engine_forkchoiceUpdatedV3", start, err)
	if err != nil {
		metrics.IncExecutionForkchoiceUpdated(metrics.ExecutionResultUnreachable)
		logger.Warn(logger.Execution, "forkchoiceUpdated failed: %v", err)
		return result, err
	}
	metrics.IncExecutionForkchoiceUpdated(strings.ToLower(result.PayloadStatus.Status))
	switch result.PayloadStatus.Status {
	case execution.StatusValid:
		d.setValidated(state.HeadBlockHash, true)
	case execution.StatusInvalid, execution.StatusInvalidBlockHash:
		d.setValidated(state.HeadBlockHash, false)
		logger.Error(logger.Execution, "execution forkchoice rejected head=%x", state.HeadBlockHash)
	}
	return result, nil
}

// observe records a call's latency and keeps the reachability gauge and the
// backoff window in step with transport failures.
func (d *ExecutionDriver) observe(method string, start time.Time, err error) {
	metrics.ObserveExecutionCall(method, time.Since(start).Seconds())
	if err == nil {
		metrics.SetExecutionReachable(true)
		return
	}
	if !execution.IsTransport(err) {
		return
	}
	metrics.SetExecutionReachable(false)
	d.mu.Lock()
	d.downUntil = time.Now().Add(executionUnreachableBackoff)
	d.mu.Unlock()
}

// unreachable backs off after transport failures to avoid a timeout per queued block.
func (d *ExecutionDriver) unreachable() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Now().Before(d.downUntil)
}

type unreachableError struct{}

func (unreachableError) Error() string { return "execution client unreachable, backing off" }

var errUnreachableBackoff = unreachableError{}

// runExecutionVerifier quarantines unresolved blocks in a bounded queue. Only
// VALID results reach BlockCh; invalid results are removed without ever entering
// fork choice. At capacity, prefer earlier blocks so missing ancestors can still
// unblock the queue. Discarded tails can be fetched again from the unchanged CL head.
func (e *Engine) runExecutionVerifier(ctx context.Context) {
	if e.Execution == nil {
		return
	}
	d := e.Execution
	ticker := time.NewTicker(executionRetryInterval)
	defer ticker.Stop()
	pending := make([]*types.SignedBlock, 0, MaxPendingBlocks)
	roots := make(map[[32]byte]bool)
	parentRequests := make(map[[32]byte]time.Time)
	discard := func(block *types.SignedBlock, root [32]byte) {
		delete(parentRequests, block.Block.ParentRoot)
		logger.Warn(logger.Execution, "execution quarantine full, discarding slot=%d root=%x for later retrieval", block.Block.Slot, root)
		// A by-root fetch must not remain marked in flight after eviction.
		// Range sync restarts from the imported head on its next peer poll.
		select {
		case e.FailedRootCh <- root:
		case <-ctx.Done():
		}
	}
	process := func(block *types.SignedBlock) bool {
		switch d.checkPayload(ctx, block) {
		case executionDeferred:
			// A head-by-root response may arrive before its ancestors. Since
			// quarantine does not enter the CL import path, request its parent
			// here; otherwise both CL and EL can wait forever for that parent.
			parent := block.Block.ParentRoot
			if !types.IsZeroRoot(parent) && !e.Store.HasState(parent) && !roots[parent] && time.Since(parentRequests[parent]) >= executionUnreachableBackoff {
				if stored := e.Store.GetSignedBlock(parent); stored != nil {
					if d.enqueue(stored) {
						parentRequests[parent] = time.Now()
					}
				} else {
					select {
					case e.FetchRootCh <- parent:
						parentRequests[parent] = time.Now()
					default:
					}
				}
			}
			return false
		case executionValid:
			select {
			case e.BlockCh <- block:
			case <-ctx.Done():
			}
		}
		delete(parentRequests, block.Block.ParentRoot)
		return true
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.forkchoiceCh:
			_, _ = d.forkchoiceUpdated(ctx, d.forkchoiceState(), nil)
		case block := <-d.verifyCh:
			if block == nil || block.Block == nil || block.Block.Body == nil {
				continue
			}
			root, err := block.Block.HashTreeRoot()
			if err != nil || roots[root] {
				continue
			}
			if !process(block) {
				if len(pending) == MaxPendingBlocks {
					// Keep receiving even when full: an arriving parent may
					// resolve every queued child. Match the CL pending buffer's
					// policy of preserving the blocks nearest the known chain.
					farthest := 0
					for i := range pending {
						if pending[i].Block.Slot > pending[farthest].Block.Slot {
							farthest = i
						}
					}
					if block.Block.Slot >= pending[farthest].Block.Slot {
						discard(block, root)
						continue
					}
					evicted := pending[farthest]
					evictedRoot, _ := evicted.Block.HashTreeRoot()
					delete(roots, evictedRoot)
					// Retry the newly admitted ancestor first, preserving the
					// order of the other retained blocks.
					copy(pending[1:farthest+1], pending[:farthest])
					pending[0] = block
					roots[root] = true
					discard(evicted, evictedRoot)
					continue
				}
				pending = append(pending, block)
				roots[root] = true
			}
		case <-ticker.C:
			if d.unreachable() {
				continue
			}
			// Round-robin retries allow a later-arriving parent to unblock a
			// child, and cap work between reads of ingress and cancellation.
			count := min(len(pending), executionRetryBatchSize)
			for i := 0; i < count && ctx.Err() == nil; i++ {
				block := pending[0]
				pending[0] = nil
				pending = pending[1:]
				if process(block) {
					root, _ := block.Block.HashTreeRoot()
					delete(roots, root)
				} else {
					pending = append(pending, block)
				}
				if d.unreachable() {
					break
				}
			}
		}
	}
}
