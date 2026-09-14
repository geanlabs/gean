package node

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// Per-call deadlines, sized to the slot phase each call serves. A forkchoice
// update is informational and must not queue behind a slow client; getPayload
// runs inside interval 0 ahead of proving and signing; newPayload gates block
// import and is the only call allowed a full execution of the payload.
const (
	executionForkchoiceTimeout  = time.Second
	executionGetPayloadTimeout  = 600 * time.Millisecond
	executionNewPayloadTimeout  = 2 * time.Second
	executionHashCacheLimit     = 4096
	executionVerifyQueueSize    = 256
	executionUnreachableBackoff = 2 * time.Second
)

// Reasons the proposal path skips a slot for want of a payload, reported under
// the proposal proof-operation counter next to the existing skip reasons.
const (
	proposalSkipNoPayload    = "no_payload"
	proposalSkipPayloadStale = "payload_stale"
)

// ExecutionDriver owns every Engine API call the node makes, so the dispatch
// loop never waits on the execution client. It is nil on a node without one.
//
// Ownership: prepare runs on its own goroutine and hands a payload id to the
// proposal worker through the mutex; verify runs on the ingress worker;
// forkchoice notifications are fire-and-forget goroutines. The root-to-hash
// cache is what lets a forkchoice update be assembled without decoding
// blocks on the dispatch loop.
type ExecutionDriver struct {
	engine       execution.Engine
	store        *store.ConsensusStore
	feeRecipient [types.AddressSize]byte

	mu        sync.Mutex
	prepared  *preparedPayload
	hashes    map[[32]byte][32]byte
	downUntil time.Time

	verifyCh chan *types.SignedBlock
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
		verifyCh:     make(chan *types.SignedBlock, executionVerifyQueueSize),
	}
}

// remember caches the execution block hash a consensus root carries. Called on
// import so forkchoice updates never read storage on the dispatch loop.
func (d *ExecutionDriver) remember(root, hash [32]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.hashes) >= executionHashCacheLimit {
		d.hashes = make(map[[32]byte][32]byte)
	}
	d.hashes[root] = hash
}

// blockHash resolves a consensus root to the execution block hash its body
// carries, falling back to storage for roots imported before this process
// started. An unknown root resolves to zero, which the execution client
// reads as "no opinion".
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
	return execution.ForkchoiceState{
		HeadBlockHash:      d.blockHash(d.store.Head()),
		SafeBlockHash:      d.blockHash(d.store.SafeTarget()),
		FinalizedBlockHash: d.blockHash(d.store.LatestFinalized().Root),
	}
}

// notifyForkchoice sends the current view without waiting for the reply.
func (d *ExecutionDriver) notifyForkchoice(state execution.ForkchoiceState) {
	go func() {
		_, _ = d.forkchoiceUpdated(context.Background(), state, nil)
	}()
}

// prepare asks the execution client to start building the payload for slot on
// top of parentRoot's execution block. The id comes back on a goroutine and
// is stashed for takePayload; a client that is syncing returns no id and the
// slot is simply not proposed.
func (d *ExecutionDriver) prepare(slot uint64, parentRoot [32]byte, state execution.ForkchoiceState, genesisTime uint64) {
	attrs := &execution.PayloadAttributes{
		Timestamp:             execution.Quantity(statetransition.ComputeTimeAtSlot(genesisTime, slot)),
		SuggestedFeeRecipient: d.feeRecipient,
		Withdrawals:           []execution.Withdrawal{},
		ParentBeaconBlockRoot: parentRoot,
	}
	go func() {
		result, err := d.forkchoiceUpdated(context.Background(), state, attrs)
		if err != nil {
			return
		}
		if result.PayloadID == nil {
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
	metrics.IncExecutionGetPayload("success")
	return payload, ""
}

// submit hands a locally built block's payload to the execution client.
// Nothing gossips our own block back to us, so this is the only path by
// which the client learns of it. It runs on the proposal worker before the
// block is handed to the dispatch loop, so by the time import moves the head
// the client already holds the execution block and the forkchoice update
// that follows lands on a block it has executed rather than one it must
// fetch from peers it does not have.
func (d *ExecutionDriver) submit(ctx context.Context, payload *types.ExecutionPayload, parentRoot [32]byte) {
	status, err := d.newPayload(ctx, payload, parentRoot)
	if err != nil {
		logger.Warn(logger.Execution, "own payload not accepted by the execution client: %v", err)
		return
	}
	metrics.IncExecutionNewPayload(strings.ToLower(status.Status))
	if status.Status != execution.StatusValid {
		logger.Warn(logger.Execution, "own payload status=%s", status.Status)
	}
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

// verify asks the execution client to execute the block's payload and decides
// whether import proceeds. VALID, SYNCING, and ACCEPTED all import: the latter
// two are the client's own optimistic answers while it lacks the parent.
// INVALID and INVALID_BLOCK_HASH reject. A client that cannot be reached is
// treated as syncing and backed off from, so consensus keeps moving while the
// reachability gauge shows the gap.
func (d *ExecutionDriver) verify(ctx context.Context, block *types.SignedBlock) bool {
	if block == nil || block.Block == nil || block.Block.Body == nil {
		return false
	}
	payload := &block.Block.Body.ExecutionPayload
	parentRoot := block.Block.ParentRoot

	status, err := d.newPayload(ctx, payload, parentRoot)
	if err != nil {
		metrics.IncExecutionNewPayload(metrics.ExecutionResultUnreachable)
		logger.Warn(logger.Execution, "newPayload unavailable slot=%d, importing optimistically: %v", block.Block.Slot, err)
		return true
	}

	metrics.IncExecutionNewPayload(strings.ToLower(status.Status))
	switch status.Status {
	case execution.StatusValid, execution.StatusSyncing, execution.StatusAccepted:
		return true
	case execution.StatusInvalid, execution.StatusInvalidBlockHash:
		metrics.IncExecutionBlocksRejected()
		reason := ""
		if status.ValidationError != nil {
			reason = *status.ValidationError
		}
		logger.Warn(logger.Execution, "execution client rejected block slot=%d status=%s %s", block.Block.Slot, status.Status, reason)
		return false
	default:
		logger.Warn(logger.Execution, "unknown newPayload status %q slot=%d, importing", status.Status, block.Block.Slot)
		return true
	}
}

func (d *ExecutionDriver) newPayload(ctx context.Context, payload *types.ExecutionPayload, parentRoot [32]byte) (execution.PayloadStatus, error) {
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

// unreachable reports whether a recent transport failure means the next call
// should be skipped rather than waited on. Without this a down client costs
// every queued block a full timeout, and a node catching up falls further
// behind for as long as the client is away.
func (d *ExecutionDriver) unreachable() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Now().Before(d.downUntil)
}

type unreachableError struct{}

func (unreachableError) Error() string { return "execution client unreachable, backing off" }

var errUnreachableBackoff = unreachableError{}

// runExecutionVerifier is the ingress worker: it serialises newPayload calls,
// which bounds the execution client's load and keeps requested blocks in the
// order they were delivered.
func (e *Engine) runExecutionVerifier(ctx context.Context) {
	if e.Execution == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case block := <-e.Execution.verifyCh:
			if !e.Execution.verify(ctx, block) {
				continue
			}
			select {
			case e.BlockCh <- block:
			case <-ctx.Done():
				return
			}
		}
	}
}
