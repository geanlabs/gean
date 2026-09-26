# Client Log Patterns

gean's patterns below are verified against `internal/` and `cmd/`. Patterns for
other clients were collected from past runs and are **unverified**; confirm them
in the logs at hand before relying on them. Client background lives in
[devnet-runner/references/clients.md](../../devnet-runner/references/clients.md).

## gean (Go)

**Log format:** `YYYY-MM-DDTHH:MM:SS.sssZ LEVEL [component] message key=value ...`
(`internal/logger`). Every line carries ANSI color codes; strip them first:

```bash
sed -E 's/\x1b\[[0-9;]*m//g' gean_0.log | grep pattern
```

Components: `[chain]`, `[validator]`, `[gossip]`, `[network]`, `[signature]`,
`[forkchoice]`, `[sync]`, `[node]`, `[state]`, `[store]`.

### Block proposal flow

```
[validator] proposing block slot=N validator=V
[chain] processing block slot=N block_root=0x... has_parent=true
[chain] block slot=N block_root=0x... parent_root=0x... proposer=V attestations=A justified_slot=J finalized_slot=F proc_time=X
[forkchoice] head slot=N head_root=0x... parent_root=0x... justified_slot=J ... finalized_slot=F ...
[validator] proposed block slot=N block_root=0x... attestations=A
```

Failures on this path log at ERROR under `[validator]` (`produce block failed`,
`sign block failed`, `merge block proof failed`) or `[network]`
(`publish block failed`).

### Block reception flow

```
[gossip] received block slot=N proposer=V block_root=0x... parent_root=0x...
[chain] processing block slot=N block_root=0x... has_parent=true|false
[chain] block slot=N ... proc_time=X                  (success)
[chain] block processing failed slot=N block_root=0x...: <err>   (ERROR)
[chain] block parent missing slot=N block_root=0x... parent_root=0x... depth=D, storing as pending
[forkchoice] head slot=N ...
```

### Head and finality

```
[forkchoice] head slot=N head_root=0x... parent_root=0x... justified_slot=J justified_root=0x... finalized_slot=F finalized_root=0x...
[forkchoice] REORG depth=D slot=N head_root=0x... parent_root=0x... (was 0x...) justified_slot=J ... finalized_slot=F ...
[forkchoice] finalized advanced slot=F root=0x...
[store] finalization stalled: finalized_slot=F current_slot=N, running periodic pruning
```

### Attestations and aggregation

```
[validator] produced attestation slot=N validator=V
[network] published attestation to network slot=N validator=V
[gossip] attestation verified: validator=V slot=N dataRoot=...
[signature] aggregate: slot=N raw=R children=C total=T proof=B bytes duration=X
[signature] aggregation worker: slot=N produced=P duration=X [skipped=...]
```

### Chain status (logged periodically)

```
  CHAIN STATUS: Current Slot: N | Head Slot: N | Behind: B
  Connected Peers:    P
  Head Block Root:    0x...
  Latest Justified:   Slot      J | Root: 0x...
  Latest Finalized:   Slot      F | Root: 0x...
  Gossip Sigs: G | Known Payloads: K | FC Nodes: N
  Topics:
  <topic>   mesh_peers=M
```

### Sync and store

```
[sync] queueing missing block block_root=0x... for batched fetch
[sync] batched fetch starting count=N
[sync] fetch exhausted for root 0x..., discarded N pending child block(s)
[sync] checkpoint sync: <url>
[store] pruning: finalized_slot=F states=S blocks=B live_chain=L gossip_sigs=G payloads=P non_canonical=N
```

## Other clients (unverified)

### zeam (Zig)

Format: `[timestamp] [level] (zeam): [module] message`, with ANSI colors.

```
[validator] packing proposer attestation for slot=X proposer=Y
[node] failed to load latest finalized state from database: error.NoFinalizedStateFound
processed block with root=0x... slot=X processing time=...
```

SSZ panics (`thread 1 panic: integer overflow`, frames in `serializedSize` or
`process_block`) produce millions of identical frames; a zeam log over ~1M lines
suggests a crash loop.

### ream (Rust, `tracing` format)

```
ream_chain_lean::service: Processing block built by Validator N slot=X block_root=0x...
ream_chain_lean::service: Attestation too far in future expected slot: X <= Y
ream_chain_lean::service: No common highest checkpoint found among connected peers
```

`Attestation too far in future`, `No state available for target 0x...`, and
`Backfill job request timed out` usually mean ream is stuck and cannot rejoin.

### ethlambda (Rust, `tracing` format)

```
ethlambda_blockchain: We are the proposer for this slot slot=X validator_id=Y
ethlambda_p2p: Published block to gossipsub slot=X proposer=Y attestation_count=A
ethlambda_blockchain: Processed new block slot=X block_root=0x... state_root=0x...
ethlambda_blockchain::store: Fork choice head updated head_slot=X head_root=0x...
ethlambda_blockchain: Failed to process block slot=X err=...
ethlambda_blockchain: Block parent missing, storing as pending slot=X parent_root=0x...
```

### lantern (C)

Format: `timestamp LEVEL [module] message`.

```
[state] imported block slot=X new_head_slot=Y head_root=0x...
[gossip] received block slot=X proposer=Y root=0x... source=gossip
[gossip] processed vote validator=X slot=Y head=0x... target=0x...@N source=0x...@M
[QUIC] handshake timeout state=client_init_sent(...)
```

QUIC handshake timeouts usually mean the peer is unreachable, not a lantern bug.

## Cross-Client Crash Detection

```bash
# Unusually large logs are often crash loops
wc -l *.log | sort -n

grep -l "panic\|fatal\|stack overflow\|segmentation" *.log
```
