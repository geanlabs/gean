# Error Classification Guide

Reference for categorizing and understanding errors in devnet logs.

## Critical Errors

Errors that indicate serious problems requiring immediate attention.

| Pattern | Meaning | Action |
|---|---|---|
| `genesis mismatch` | Nodes have different genesis configurations | Check genesis consistency across nodes |
| `panic` / `fatal` / `thread panic` | Client crash | Check stack trace, file bug report |
| `stack overflow` / `runtime error: stack` | Crash from infinite recursion | Check SSZ decoder, file bug |
| `database corruption` | Data directory corrupted | Clear data directory and restart |
| `OutOfMemory` in block deserialization | Block format incompatibility between clients | Check SSZ schema versions |
| `MessageTooLarge` (publishing own block) | Producer-side block bloat bug | Check the producer's block builder |
| `snappy decoded len NNNNNNNN exceeds max 10485760` | Receiving an oversized block | Identify the producer (peer ID), file bug |
| `xmss_aggregate.rs panic` | Missing signature aggregation prover files | Ensure prover files are in correct location |

## Expected/Benign Messages

Messages that look like errors but are actually normal or harmless.

| Pattern | Meaning | Why It's OK |
|---|---|---|
| `Error response from daemon: manifest unknown` | Docker image tag not found in remote registry | Docker falls back to local image; only an issue if no local image exists |
| `failed to load latest finalized state from database: NoFinalizedStateFound` | Fresh start, no previous state | Normal for new devnet runs |
| `HandshakeTimedOut` to ports of unconfigured nodes | Connection attempt to node that doesn't exist | Expected when validator config has fewer nodes than the network expects |
| `[QUIC] handshake timeout` (lantern) to dead peers | Peer disappeared, lantern still trying | Cosmetic; back-off should kick in |
| `TODO precompute poseidons in parallel + SIMD` | Performance optimization not yet implemented | Code TODOs, not runtime errors |
| `TODO optimize open_columns when no shifted F columns` | AIR proof optimization not yet implemented | Code TODOs, not runtime errors |

## Medium Severity

Issues that may indicate problems but don't immediately break consensus.

| Pattern | Meaning | Action |
|---|---|---|
| `Failed to decode snappy-framed RPC request` | Protocol/encoding mismatch between clients | Check libp2p versions and snappy compression settings |
| `No callback found for request_id` | Response received for unknown request | May indicate internal state tracking issue |
| `UnexpectedEof` | Incomplete message received | Check network stability and message size limits |
| `Proposer signature verification failed` | Block has invalid proposer signature | Check if block is genuinely invalid or validation bug |
| `Invalid signatures for block` | Block has invalid attestation signatures | Check XMSS signature aggregation |
| `signature verification failed` | Generic signature validation failure | Check which signature type failed |
| `Unknown head block` | Attestation references block client doesn't have | May indicate fork or missing block |
| `Unknown target block` | Attestation target block not found | May indicate fork or missing block |
| `Block parent missing` | Received block but parent not available | Client will try to fetch parent |
| `block parent missing slot=N ... depth=D, storing as pending` (gean) | gean's pending block cache absorbing orphan | Normal during sync; problematic if depth grows |
| `Attestation too far in future` (ream) | ream is too far behind to validate the attestation | ream is stuck — sync recovery issue |
| `No state available for target 0x...` (ream) | Missing fork choice target state | ream cannot follow a divergent branch |
| `No common highest checkpoint found among connected peers` (ream) | Backfill cannot seed | ream sync deadlocked |

## Connection Timeouts

Connection timeouts to specific ports usually mean the node for that port was
never started, was paused, or crashed.

**Identifying the node:**
Check the `validator-config.yaml` file in the network directory:
- `lean-quickstart/local-devnet/genesis/validator-config.yaml`
- `lean-quickstart/ansible-devnet/genesis/validator-config.yaml`

Each node entry has an `enrFields.quic` port.

**If you see HandshakeTimedOut to certain ports but those nodes were never started, this is expected.**

If a node was running and now isn't, that node likely crashed — check its log
for panics.

## State Transition Errors

### State Root Mismatch During Proposal

If you see this pattern (in any client):
```
We are the proposer for this slot slot=N validator_id=X
...
Failed to process block slot=N err=State transition failed: state root mismatch
Published block slot=N validator_id=X
```

This indicates a **block building bug**, not a consensus issue:
- The proposer builds a block with one state root in the header
- When verifying its own block, it computes a different state root
- The block is published anyway (bug: should not publish invalid blocks)
- Other nodes will also fail to process it with the same mismatch

**Key diagnostic:** If all nodes compute the **same** state root (but different
from the block header), the state transition is deterministic — the bug is in
how the block header's state root is computed during block building.

## Interoperability Issues

When analyzing multi-client devnets, watch for:

1. **Status exchange failures** — clients failing to exchange status messages
2. **Block/attestation propagation** — messages not reaching all clients
3. **Encoding mismatches** — snappy/SSZ encoding differences
4. **Timing issues** — slot timing drift between clients
5. **Block format incompatibility** — SSZ schema differences causing
   deserialization failures (look for `OutOfMemory` errors)
6. **Stale containers** — containers from previous runs causing genesis mismatch
   (look for `UnknownSourceBlock`)
7. **Signature validation disagreements** — clients disagree on signature
   validity (indicates bug in proposer or validator)
8. **Oversized block cascades** — one producer's bloated blocks crash multiple
   peers; look for `MessageTooLarge` (producer side) and `snappy decoded len
   exceeds max` (receiver side) with the same byte size

## gean-Specific Patterns

### Healthy gean

- `attestations=` count in [chain] block log stays low (typically 1-5 with the
  per-validator refactor)
- `proc_time=` < 200ms for normal blocks
- `Behind: 0` or `Behind: 1` in chain status
- `has_parent=true` for almost all incoming blocks
- `[forkchoice] head` updates each slot

### Unhealthy gean (regressions to watch for)

| Symptom | Likely cause |
|---|---|
| `attestations=NN` (e.g., 50+) | per-validator refactor regressed |
| `MessageTooLarge` in gean's own log | block builder regressed |
| `proc_time=Xs` (seconds, not ms) | aggregation slow / CPU pressure |
| `has_parent=false` repeatedly | sync recovery issue |
| `Behind:` growing without bound | gean falling behind wall clock |
| `[sync] fetch exhausted for root` (many) | peer dropping out, batched fetch failing |

## Searching for Errors

```bash
# Generic error search
grep -i "error\|ERROR" *.log | grep -vE "manifest unknown|HandshakeTimedOut|NoFinalizedStateFound" | head -50

# Search for specific critical patterns
grep -i "genesis mismatch\|panic\|fatal\|stack overflow" *.log

# Block bloat regression check (any client)
grep -i "MessageTooLarge\|snappy decoded len.*exceeds max" *.log

# Client-specific error patterns
grep "block processing failed" gean_0.log
grep "Failed to process block" ethlambda_0.log
grep "Invalid signatures" qlean_0.log 2>/dev/null  # if qlean was included
grep "signature verification failed" lantern_0.log
grep "Failed to handle process" ream_0.log
```
