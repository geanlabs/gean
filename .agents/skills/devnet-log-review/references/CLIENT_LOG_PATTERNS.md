<!-- Adapted from ethlambda (github.com/lambdaclass/ethlambda), MIT License, Copyright (c) 2025 Lambdaclass. -->

# Client-Specific Log Patterns

Reference guide for log formats and key patterns across different lean consensus clients.

## gean (Go)

**Log format:** `timestamp LEVEL [component] message`, e.g.
`2026-01-01T00:00:00.000Z INFO [chain] block slot=5 ...`. Components include `[chain]`,
`[forkchoice]`, `[validator]`, `[signature]`, `[gossip]`, `[network]`, `[sync]`, `[store]`, `[node]`.

### Block proposal
```
INFO [validator] proposing block slot=X validator=Y
INFO [validator] proposed block slot=X block_root=0x... attestations=N
WARN [validator] discarding proposal slot=X: head moved head_slot=Y head=0x... parent=0x...
```

### Block import and fork choice
```
INFO [gossip] received block slot=X proposer=Y block_root=0x... parent_root=0x...
INFO [chain] processing block slot=X block_root=0x... has_parent=true
INFO [chain] block slot=X block_root=0x... parent_root=0x... proposer=Y attestations=N justified_slot=J finalized_slot=F proc_time=...
INFO [forkchoice] head slot=X head_root=0x... parent_root=0x... justified_slot=J ... finalized_slot=F ...
INFO [forkchoice] finalized advanced slot=X root=0x...
```

### Aggregation (aggregator nodes)
```
INFO [signature] aggregate: slot=X raw=N children=M total=T proof=B bytes duration=...
WARN [signature] aggregation skipped: prover unavailable slot=X
```

### Missing parents and sync
```
INFO [sync] queueing missing block block_root=0x... for batched fetch
INFO [sync] sync: fetching range from peer <id> start_slot=X count=N peer_head=H
WARN [sync] fetch exhausted for root 0x..., discarded N pending child block(s)
```

### Errors
```
ERROR [chain] block processing failed slot=X block_root=0x...: <reason>
ERROR [signature] aggregated attestation verification failed: <reason>
WARN [forkchoice] justified root 0x... unknown to fork choice; head cannot advance past it
```

### Chain status
Every few slots gean prints a multi-line block:
```
  CHAIN STATUS: Current Slot: X | Head Slot: Y | Behind: Z
  Latest Justified:   Slot      J | Root: 0x...
  Latest Finalized:   Slot      F | Root: 0x...
```

### Counting blocks
```bash
grep -c "\[validator\] proposed block" gean_0.log   # blocks this node built
grep -c "\[chain\] block slot=" gean_0.log          # blocks this node imported
```

## zeam (Zig)

**Log format:** `[timestamp] [level] (zeam): [module] message`

**Key characteristics:**
- Color codes in output (ANSI escape sequences)
- Key modules: `[node]`, `[network]`, `[consensus]`

**Common patterns:**
```
[validator] packing proposer attestation for slot=X proposer=Y
[database] initializing RocksDB
[node] failed to load latest finalized state from database: error.NoFinalizedStateFound
```

## ream (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key characteristics:**
- Uses tracing crate format
- Key modules: `ream_p2p::network::lean`, `ream_blockchain`

**Common patterns:**
```
ream_p2p::network::lean: Connected to peer: PeerId("...")
ream_blockchain: Processing block slot=X
```

## ethlambda (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key modules:**
- `ethlambda`
- `ethlambda_blockchain`
- `ethlambda_p2p`
- `ethlambda_p2p::gossipsub`

**Key patterns:**

### Block Proposal
```
ethlambda_blockchain: We are the proposer for this slot slot=X validator_id=Y
ethlambda_blockchain: Published block slot=X validator_id=Y
ethlambda_p2p: Published block to gossipsub slot=X proposer=Y
```

### Attestations
```
ethlambda_blockchain: Published attestation slot=X validator_id=Y
ethlambda_p2p::gossipsub::handler: Received new attestation from gossipsub, sending for processing slot=X validator=Y
```

### Block Processing
```
ethlambda_p2p::gossipsub::handler: Received new block from gossipsub, sending for processing slot=X
ethlambda_blockchain::store: Processed new block slot=X block_root=0x... state_root=0x...
ethlambda_blockchain: Block processed successfully slot=X
```

### Errors
```
ethlambda_blockchain: Failed to process block slot=X err=Proposer signature verification failed
ethlambda_blockchain: Failed to build block slot=X err=...
ethlambda_blockchain: Block parent missing, storing as pending slot=X parent_root=0x... block_root=0x...
ethlambda_blockchain: Failed to process gossiped attestation err=Unknown head block: 0x...
```

### Counting Blocks
Each block proposal generates TWO "Published block" log lines:
1. `ethlambda_blockchain: Published block slot=X validator_id=Y` (block built)
2. `ethlambda_p2p: Published block to gossipsub slot=X proposer=Y` (block broadcast)

To count accurately:
```bash
# Count only blockchain module's log (one per block)
sed 's/\x1b\[[0-9;]*m//g' ethlambda_0.log | grep "ethlambda_blockchain: Published block" | wc -l

# Or count "Published block to gossipsub" (also one per block)
sed 's/\x1b\[[0-9;]*m//g' ethlambda_0.log | grep "Published block to gossipsub" | wc -l
```

### Attestation Math
Each validator attests to all slots except slots where they're the proposer. With round-robin and N validators over S slots, each validator publishes approximately `S - (S/N)` attestations.

## grandine (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key modules:**
- `validator`
- `block_producer`
- `validator_config`

**Key patterns:**
```
CHAIN STATUS: Current Slot: X | Head Slot: Y | Behind: Z
Head Block Root: 0xabc...
Using parent root for block proposal parent_root=0x...
Finalized Slot: X
```

**Checking chain head:**
```bash
grep "CHAIN STATUS\|Head Block Root" grandine_0.log
```

## lantern (Rust)

**Log format:** `timestamp LEVEL [module] message`

**Key characteristics:**
- Brackets around module names: `[state]`, `[gossip]`, `[network]`

**Key patterns:**
```
[state] imported block slot=X new_head_slot=Y head_root=0x...
[gossip] rejected vote validator=X slot=Y head=0x... reason=unknown head
[gossip] received block slot=X proposer=Y root=0x... source=gossip
[state] signature verification failed slot=X root=0x...
```

## qlean (C++)

**Log format:** `date time log-level module message`

**Key characteristics:**
- No colons separating fields, spaces only
- Date format: `YY.MM.DD HH:MM:SS.microseconds`

**Key patterns:**
```
BlockStorage  Add slot-to-hash for 0x... @ X
Networking  Received block 0x... @ X parent=0x... from peer=...
BlockStorage  Added block 0x... @ X as child of abc1…2345
ForkChoice  Invalid signatures for block 0x... @ X
BlockTree  Finalized block 0x... @ X
ForkChoice  🔒 Finalized block: 0x... @ X
Networking  ❌ Error importing block=0x... @ X: Invalid attestation
```

**Checking parent relationships:**
```bash
grep "Received block.*parent=" qlean_0.log
grep "Added block.*as child of" qlean_0.log
```

## ANSI Color Code Handling

Many clients output ANSI escape sequences for terminal colors. Strip them before grepping:

```bash
# Strip ANSI codes
sed 's/\x1b\[[0-9;]*m//g' logfile.log | grep pattern
```

Without stripping, patterns may not match correctly.
