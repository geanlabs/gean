# Client-Specific Log Patterns

Reference guide for log formats and key patterns across different lean consensus clients.

## gean (Go)

**Log format:** `timestamp LEVEL [component] message key=value key=value`

**Components:**
- `[node]` - Main node orchestration
- `[validator]` - Validator duties
- `[network]` - P2P networking
- `[gossip]` - Gossipsub messages
- `[reqresp]` - Request/Response protocol

**Key patterns:**

### Block Processing
```
[gean] block accepted slot=X proposer=Y block_root=0x... parent_root=0x...
[gossip] received block via gossip slot=X proposer=Y block_root=0x...
[gean] synced block slot=X block_root=0x... peer_id=... progress=N/N
```

### Sync
```
[node] starting sync walk peer_id=... peer_head_slot=X our_head_slot=Y gap_slots=Z
[node] blocks_by_root batch request peer_id=... roots_count=N
[node] blocks_by_root batch response received peer_id=... requested=N received=N duration_ms=X
[node] sync with peer completed peer_id=... blocks_synced=N new_head_slot=X
[node] skipping validator duties while behind peers slot=X head_slot=Y max_peer_head=Z
```

### Finalization
```
[gean] slot slot=X head_slot=Y finalized_slot=Z justified_slot=W peers=N behind_peers=false
```

### Errors
```
[WRN] parent state missing for gossip block slot=X block_root=0x...
[WRN] sync block rejected slot=X block_root=0x... err=...
[ERR] status exchange failed peer_id=... err=...
```

### Peer Connections
```
[network] peer connected peer_id=... direction=inbound peers=N
[network] peer disconnected peer_id=... direction=outbound peers=N
```

### Counting Blocks
```bash
# Count blocks accepted via gossip
grep "block accepted" gean_0.log | wc -l

# Count blocks synced
grep "synced block" gean_0.log | wc -l

# Count BlocksByRoot requests (should be batched after fix)
grep "blocks_by_root" gean_0.log | wc -l
```

## zeam (Zig)

**Log format:** `[timestamp] [level] (zeam): [module] message`

**Key patterns:**
```
[validator] packing proposer attestation for slot=X proposer=Y
[database] initializing RocksDB
```

## ream (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key patterns:**
```
ream_p2p::network::lean: Connected to peer: PeerId("...")
ream_blockchain: Processing block slot=X
```

## ethlambda (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key patterns:**
```
ethlambda_blockchain: We are the proposer for this slot slot=X validator_id=Y
ethlambda_blockchain: Published block slot=X validator_id=Y
ethlambda_p2p: Published block to gossipsub slot=X proposer=Y
```

**Counting blocks:**
```bash
# Count only blockchain module's log (one per block)
sed 's/\x1b\[[0-9;]*m//g' ethlambda_0.log | grep "ethlambda_blockchain: Published block" | wc -l
```

## grandine (Rust)

**Log format:** `timestamp LEVEL module: message`

**Key patterns:**
```
CHAIN STATUS: Current Slot: X | Head Slot: Y | Behind: Z
Head Block Root: 0xabc...
Finalized Slot: X
```

## lantern (Rust)

**Log format:** `timestamp LEVEL [module] message`

**Key patterns:**
```
[state] imported block slot=X new_head_slot=Y head_root=0x...
[gossip] rejected vote validator=X slot=Y head=0x... reason=unknown head
```

## qlean (C++)

**Log format:** `date time log-level module message`

**Key patterns:**
```
BlockStorage  Add slot-to-hash for 0x... @ X
ForkChoice  Invalid signatures for block 0x... @ X
ForkChoice  🔒 Finalized block: 0x... @ X
```

## ANSI Color Code Handling

Many clients output ANSI escape sequences for terminal colors. Strip them before grepping:

```bash
# Strip ANSI codes
sed 's/\x1b\[[0-9;]*m//g' logfile.log | grep pattern
```
