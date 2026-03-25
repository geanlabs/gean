# Error Classification

Categorization of errors and warnings encountered during devnet runs.

## Severity Levels

| Level | Description | Action Required |
|-------|-------------|----------------|
| 🔴 CRITICAL | Consensus broken, genesis mismatch, panics | Investigate immediately |
| 🟡 WARNING | Non-fatal issues, missing data | Monitor and investigate |
| 🟢 INFO | Expected behavior, TODOs | None |

## Critical Errors

### Genesis Mismatch
```
FATAL|panic|genesis
```
**Cause:** Different clients have inconsistent genesis state.

### Database Corruption
```
database corruption|db error|fatal
```
**Cause:** Persistent storage failure or corruption.

### Proposer Index Out of Range
```
proposer index.*out of range|invalid proposer
```
**Cause:** State validator count mismatch between clients.

## Warning-Level Issues

### Parent State Missing
```
parent state missing|cached pending block awaiting parent
```
**Cause:** Block received before parent (normal in syncing).
**Action:** Normal during catch-up, should resolve after sync.

### Block Rejected During Sync
```
sync block rejected|rejected gossip block
```
**Cause:** Block validation failed (signature, state transition).
**Action:** Check for fork or signature verification issues.

### Status Exchange Failed
```
status exchange failed|failed to connect to bootnode
```
**Cause:** Network connectivity issues.
**Action:** Check peer connections and network config.

### Behind Peers
```
skipping validator duties while behind peers
```
**Cause:** Node is catching up from checkpoint or restart.
**Action:** Normal behavior, should resolve after sync completes.

## Benign Messages (No Action)

These patterns are expected and should be filtered from error counts:

### Connection Timeouts
```
connection timed out|HandshakeTimedOut
```
**Note:** Expected when connecting to nodes not running.

### TODOs
```
TODO|FIXME|not yet implemented
```
**Note:** Known unimplemented features.

### Peer Disconnections
```
peer disconnected|Connection closed
```
**Note:** Normal peer churn in P2P networks.

### Parent Block Pending
```
parent state missing.*pending|caching pending block
```
**Note:** Normal during sync, block will be processed when parent arrives.

## Interoperability Issues

### Encoding Mismatches
```
failed to decode|invalid ssz|marshal error
```
**Cause:** SSZ encoding differences between clients.

### Block Root Mismatch
```
block root mismatch|unexpected block root
```
**Cause:** Fork or state root disagreement.

### Attestation Target Unknown
```
unknown head block|target block not found
```
**Cause:** Block not yet propagated or fork difference.

### Signature Verification Failure
```
signature verification failed|invalid signature
```
**Cause:** Cryptographic disagreement or encoding issue.

## Debugging Checklist

1. **Genesis consistent?** Check genesis state across all logs
2. **Peers connected?** Verify all nodes have peer connections
3. **Blocks propagating?** Check block acceptance across clients
4. **Finalization advancing?** Monitor finalized_slot progression
5. **Sync complete?** Check for "behind peers" warnings
