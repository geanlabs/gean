# Client-Specific Log Patterns

Reference guide for log formats and key patterns across the lean consensus
clients used in gean's multi-client devnet (gean, zeam, ream, ethlambda, lantern).

## gean (Go)

**Log format:** `YYYY-MM-DDTHH:MM:SS.sssZ LEVEL [module] message key=value ...`

**Key characteristics:**
- ANSI color codes for level + module
- Modules: `[chain]`, `[gossip]`, `[validator]`, `[network]`, `[forkchoice]`,
  `[signature]`, `[sync]`, `[store]`
- Structured fields use `key=value`
- Periodic chain status box with `Latest Justified` / `Latest Finalized`

### Block proposal flow

```
[validator] proposing block slot=N validator=V
[chain] block slot=N block_root=0x... parent_root=0x... proposer=V
       attestations=A justified_slot=J finalized_slot=F proc_time=Xms
[forkchoice] head slot=N head_root=0x... ... justified_slot=J finalized_slot=F
[validator] proposed block slot=N block_root=0x... attestations=A
[signature] aggregate: slot=N sigs=S validators=[...] proof=B bytes duration=Xms
```

### Block reception flow

```
[gossip] received block slot=N proposer=V block_root=0x... parent_root=0x...
[chain] processing block slot=N block_root=0x... has_parent=true|false
[chain] block slot=N ... proc_time=Xms
[forkchoice] head slot=N ...
```

### Attestations

```
[gossip] attestation verified: validator=V slot=N dataRoot=0x...
[validator] produced attestation slot=N validator=V
[network] published attestation to network slot=N validator=V
```

### Chain status (logged periodically)

```
+===============================================================+
  CHAIN STATUS: Current Slot: N | Head Slot: N | Behind: B
+---------------------------------------------------------------+
  Connected Peers:    P
+---------------------------------------------------------------+
  Head Block Root:    0x...
  Parent Block Root:  0x...
  State Root:         0x...
+---------------------------------------------------------------+
  Latest Justified:   Slot     J | Root: 0x...
  Latest Finalized:   Slot     F | Root: 0x...
+---------------------------------------------------------------+
  Gossip Sigs: G | Known Payloads: K | States: S | FC Nodes: N
+---------------------------------------------------------------+
  Topics:
  /leanconsensus/devnet0/block/ssz_snappy             mesh_peers=M
  /leanconsensus/devnet0/aggregation/ssz_snappy       mesh_peers=M
  /leanconsensus/devnet0/attestation_0/ssz_snappy     mesh_peers=M
+===============================================================+
```

### Sync events

```
[sync] queueing missing block block_root=0x... for batched fetch
[sync] batched fetch starting count=N
[sync] fetch exhausted for root 0x..., discarded N pending child block(s)
[sync] checkpoint sync: <url>
[sync] requesting missing block block_root=0x... from network
```

### Store events

```
[store] pruning: finalized_slot=F states=S blocks=B live_chain=L gossip_sigs=G payloads=P non_canonical=N
```

### Counting blocks

```bash
# Proposed (one per gean block proposal)
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep -c "\[validator\] proposed block"

# Processed (one per block applied to state — own + others)
sed 's/\x1b\[[0-9;]*m//g' gean_0.log | grep -c "\[chain\] block slot="
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
processed block with root=0x... slot=X processing time=...
```

**Important crash signature (seen in past incidents):**
```
thread 1 panic: integer overflow
.../ssz/src/lib.zig:N: ... in serializedSize__anon_NNNNN (zeam)
```
or
```
.../utils/src/ssz.zig:NN: ... in process_block (zeam)
```

These cause unbounded recursion / stack overflow logs (millions of identical
frames). If a zeam_0.log is unusually large (>1M lines), suspect a crash loop.

## ream (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key characteristics:**
- Uses `tracing` crate format
- Key modules: `ream_p2p::network::lean`, `ream_blockchain`, `ream_chain_lean`

**Common patterns:**
```
ream_p2p::network::lean: Connected to peer: PeerId("...")
ream_chain_lean::service: Processing block built by Validator N slot=X block_root=0x...
ream_chain_lean::service: Failed to handle process attestation message: ...
ream_chain_lean::service: Attestation too far in future expected slot: X <= Y
ream_chain_lean::service: No common highest checkpoint found among connected peers
```

**Known weakness:** ream's sync recovery is fragile. Watch for:
- `Attestation too far in future`
- `No state available for target 0x...`
- `Backfill job request timed out`

These usually indicate ream is stuck and cannot rejoin a divergent branch.

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
ethlambda_p2p: Published block to gossipsub slot=X proposer=Y attestation_count=A
```

### Attestations
```
ethlambda_blockchain: Published attestation slot=X validator_id=Y
ethlambda_p2p::gossipsub::handler: Published attestation to gossipsub slot=X validator=Y
ethlambda_blockchain: Skipping attestation for proposer slot=X
```

### Block Processing
```
ethlambda_blockchain::store: Fork choice head updated head_slot=X head_root=0x... ...
ethlambda_blockchain: Processed new block slot=X block_root=0x... state_root=0x...
```

### Errors
```
ethlambda_blockchain: Failed to process block slot=X err=...
ethlambda_blockchain: Block parent missing, storing as pending slot=X parent_root=0x...
ethlambda_p2p::swarm_adapter: Swarm adapter: publish failed err=MessageTooLarge
```

**Known regression (block bloat bug):** ethlambda's `build_block` greedily
accumulates attestations without a per-validator dedup or size cap. During
stalls, blocks can grow to ~12 MB and trigger MessageTooLarge errors. Watch for:

```
Swarm adapter: publish failed err=MessageTooLarge
gossipsub block decompression failed: uncompressed size NNNNNNNN exceeds maximum 10485760
Block fetch failed after max retries
```

This is the same bug gean had before commit `62454aa` (per-validator latest-vote
selection).

## lantern (C)

**Log format:** `timestamp LEVEL [module] message`

**Key characteristics:**
- Brackets around module names: `[state]`, `[gossip]`, `[network]`, `[QUIC]`
- Most reliable client in our network — rarely the source of bugs

**Key patterns:**
```
[state] imported block slot=X new_head_slot=Y head_root=0x...
[gossip] received block slot=X proposer=Y root=0x... source=gossip
[gossip] published attestation validator=X slot=Y
[gossip] processed vote validator=X slot=Y head=0x... target=0x...@N source=0x...@M
[signature] aggregation verify start count=N epoch=E proof_len=N
[reqresp] chunk payload too large=N peer=...
[QUIC] handshake timeout state=client_init_sent(...)
```

**Note:** lantern's QUIC handshake timeouts are usually a peer being unreachable
(dead listener), not a lantern bug. Lantern keeps redialing dead peers.

## ANSI Color Code Handling

Many clients output ANSI escape sequences for terminal colors. Strip them before
grepping:

```bash
# Strip ANSI codes
sed 's/\x1b\[[0-9;]*m//g' logfile.log | grep pattern
```

Without stripping, patterns may not match correctly.

## Cross-Client Crash Detection

Quick check for crashed clients (unusually large logs are often crash loops):

```bash
# Sort logs by size — anything > 1M lines is suspect
wc -l *.log | sort -n

# Find panic / fatal patterns
grep -l "panic\|fatal\|stack overflow\|segmentation" *.log

# zeam-specific (most common crash family in our network)
grep -m 1 "thread.*panic\|process_block\|serializedSize" zeam_*.log
```
