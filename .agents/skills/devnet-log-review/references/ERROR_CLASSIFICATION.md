<!-- Adapted from ethlambda (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Error Classification Guide

Reference for categorizing and understanding errors in devnet logs.

## Critical Errors

Errors that indicate serious problems requiring immediate attention.

| Pattern | Meaning | Action |
|---------|---------|--------|
| `genesis mismatch` | Nodes have different genesis configurations | Check genesis.json consistency across nodes |
| `panic` / `fatal` | Client crash | Check stack trace, file bug report |
| `database corruption` | Data directory corrupted | Clear data directory and restart |
| `OutOfMemory` in block deserialization | Block format incompatibility between clients | Check SSZ schema versions |
| `xmss_aggregate.rs panic` | Missing signature aggregation prover files | Ensure prover files are in correct location |

## Expected/Benign Messages

Messages that look like errors but are actually normal or harmless.

| Pattern | Meaning | Why It's OK |
|---------|---------|-------------|
| `Error response from daemon: manifest unknown` | Docker image tag not found in remote registry | Docker falls back to local image; only an issue if no local image exists |
| `failed to load latest finalized state from database: NoFinalizedStateFound` | Fresh start, no previous state | Normal for new devnet runs |
| `HandshakeTimedOut` to ports of unconfigured nodes | Connection attempt to node that doesn't exist | Expected when validator config has fewer nodes than the network expects |
| `TODO precompute poseidons in parallel + SIMD` | Performance optimization not yet implemented | Code TODOs, not runtime errors |
| `TODO optimize open_columns when no shifted F columns` | AIR proof optimization not yet implemented | Code TODOs, not runtime errors |

## Medium Severity

Issues that may indicate problems but don't immediately break consensus.

| Pattern | Meaning | Action |
|---------|---------|--------|
| `Failed to decode snappy-framed RPC request` | Protocol/encoding mismatch between clients | Check libp2p versions and snappy compression settings |
| `No callback found for request_id` | Response received for unknown request | May indicate internal state tracking issue |
| `UnexpectedEof` | Incomplete message received | Check network stability and message size limits |
| `Proposer signature verification failed` | Block has invalid proposer signature | Check if block is genuinely invalid or validation bug |
| `Invalid signatures for block` | Block has invalid attestation signatures | Check XMSS signature aggregation |
| `signature verification failed` | Generic signature validation failure | Check which signature type failed |
| `Unknown head block` | Attestation references block client doesn't have | May indicate fork or missing block |
| `Unknown target block` | Attestation target block not found | May indicate fork or missing block |
| `Block parent missing` | Received block but parent not available | Client will try to fetch parent |

## Connection Timeouts

Connection timeouts to specific ports usually mean the node for that port was never started.

**Identifying the node:**
Check the `validator-config.yaml` file in the network directory:
- `local-devnet/genesis/validator-config.yaml`
- `ansible-devnet/genesis/validator-config.yaml`

Each node entry has an `enrFields.quic` port.

**If you see HandshakeTimedOut to certain ports but those nodes were never started, this is expected.**

## State Transition Errors

### State Root Mismatch During Proposal

If you see this pattern:
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

**Key diagnostic:** If all nodes compute the **same** state root (but different from the block header), the state transition is deterministic - the bug is in how the block header's state root is computed during block building.

## Interoperability Issues

When analyzing multi-client devnets, watch for:

1. **Status exchange failures** - clients failing to exchange status messages
2. **Block/attestation propagation** - messages not reaching all clients
3. **Encoding mismatches** - snappy/SSZ encoding differences
4. **Timing issues** - slot timing drift between clients
5. **Block format incompatibility** - SSZ schema differences causing deserialization failures (look for `OutOfMemory` errors)
6. **Stale containers** - containers from previous runs causing genesis mismatch (look for `UnknownSourceBlock`)
7. **Signature validation disagreements** - clients disagree on signature validity (indicates bug in proposer or validator)

## Searching for Errors

```bash
# Generic error search
grep -i "error\|ERROR" *.log | grep -v "no callback\|manifest unknown" | head -50

# Search for specific patterns
grep -i "genesis mismatch\|panic\|fatal" *.log

# Client-specific error patterns
grep "Failed to process block" ethlambda_0.log
grep "block processing failed" gean_0.log
grep "Invalid signatures" qlean_0.log
grep "signature verification failed" lantern_0.log
```
