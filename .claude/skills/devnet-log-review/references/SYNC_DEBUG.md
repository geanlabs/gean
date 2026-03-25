# Sync Debugging

Guide to debugging sync issues and the BlocksByRoot request storm fix.

## The Problem: Request Storm

During the 12-client interop, gean sent **300+ individual BlocksByRoot requests** to peer Grandine, overwhelming it and causing stalls.

### Root Cause
The original sync logic requested blocks one-at-a-time in a loop:
```go
for i := 0; i < maxSyncDepth; i++ {
    blocks, err := reqresp.RequestBlocksByRoot(ctx, peer, [][32]byte{nextRoot})
    // Process one block, continue
}
```

### The Fix: Batched Requests

The fix implements two-phase sync:
1. **Phase 1**: Walk backwards collecting all needed roots (without requesting)
2. **Phase 2**: Single batched request with all roots

```go
// Phase 1: Collect all roots
var neededRoots [][32]byte
for i := 0; i < maxSyncDepth; i++ {
    if n.FC.HasState(nextRoot) { break }
    neededRoots = append(neededRoots, nextRoot)
    // Just get parent root, don't fetch block
    // ...
}

// Phase 2: Single batched request
blocks, err := reqresp.RequestBlocksByRoot(ctx, peer, neededRoots)
```

**Result:** 300+ requests → 1 batch request

## Detecting Sync Issues

### gean Logs to Check

```bash
# Check sync walk progress
grep "starting sync walk" gean_0.log

# Check batch requests
grep "batch request" gean_0.log

# Check batch responses
grep "batch response" gean_0.log

# Count BlocksByRoot requests (should be low after fix)
grep "blocks_by_root" gean_0.log | wc -l
```

### Before the Fix (300+ requests)
```
[INF] blocks_by_root requesting for parent chain peer_id=... root=0x... walk_depth=1
[INF] blocks_by_root requesting for parent chain peer_id=... root=0x... walk_depth=2
[INF] blocks_by_root requesting for parent chain peer_id=... root=0x... walk_depth=3
... (repeats 300 times)
```

### After the Fix (1 batch request)
```
[INF] starting sync walk peer_id=... gap_slots=300
[INF] blocks_by_root batch request peer_id=... roots_count=300
[INF] blocks_by_root batch response received peer_id=... requested=300 received=300
```

## Deduplication

Even with batched requests, deduplication prevents repeated syncs:

```go
// 30-second deduplication window
if !globalSyncDedup.shouldRequest(root) {
    break  // Skip already-requested root
}
```

## Behind Peers Status

If node keeps falling behind peers:

```bash
# Check "behind peers" occurrences
grep "skipping validator duties while behind peers" gean_0.log | wc -l

# Check peer head comparison
grep "max_peer_head" gean_0.log | tail -10
```

## Metrics for Sync Monitoring

### Key Metrics (leanMetrics)
- `lean_sync_gap_slots` - Slots behind peers
- `lean_sync_blocks_downloaded_total` - Blocks synced
- `lean_blocksbyroot_requests_total` - Request count (should be low)
- `lean_blocksbyroot_response_duration_seconds` - Response time

## Common Sync Issues

### 1. Parent State Missing
```
[WRN] parent state missing for gossip block slot=X
```
**Cause:** Block received before parent.
**Fix:** Normal during sync, should resolve after catch-up.

### 2. Sync Not Completing
```
[INF] starting sync walk peer_id=... gap_slots=500
(no completion log)
```
**Cause:** Sync failed or peer disconnected.
**Debug:** Check for `blocks_by_root failed` or peer connection logs.

### 3. Peer Not Syncing
```
[DBG] peer behind us, skipping peer_id=... peer_head_slot=5 our_head_slot=10
```
**Cause:** Peer is slower than us (expected).
**Note:** Not a problem unless we're supposed to sync from them.

## Recovery Patterns

### After Checkpoint Sync
1. Node starts at checkpoint state
2. Gap to current head calculated
3. Batch sync fills the gap
4. Normal gossip follows

### After Restart
1. State restored from database
2. Check if behind peers
3. If yes, run initial sync
4. Resume normal operation

## Best Practices

1. **Use checkpoint sync** for fast startup
2. **Batch requests** to reduce peer load
3. **Deduplicate** to prevent redundant syncs
4. **Monitor metrics** for sync health
5. **Log sync progress** for debugging
