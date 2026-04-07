# Validator Config Reference

Full schema and configuration guide for
`lean-quickstart/local-devnet/genesis/validator-config.yaml`.

## Full Schema

```yaml
shuffle: roundrobin              # Proposer selection algorithm (roundrobin = deterministic turns)
deployment_mode: local           # 'local' (localhost) or 'ansible' (remote servers)

config:
  activeEpoch: 18                # Log2 of active signing epochs for hash-sig keys (2^18)
  keyType: "hash-sig"            # Post-quantum signature scheme

validators:
  - name: "gean_0"               # Node identifier: <client>_<index>
    privkey: "bdf953adc..."      # 64-char hex P2P private key (libp2p identity)
    enrFields:
      ip: "127.0.0.1"            # Node IP (127.0.0.1 for local, real IP for ansible)
      quic: 9008                 # QUIC/UDP port for P2P communication
    metricsPort: 8088            # HTTP port exposed by the node (see note below)
    count: 1                     # Number of validator indices assigned to this node
```

## Field Reference

| Field | Required | Description |
|---|---|---|
| `shuffle` | Yes | Proposer selection algorithm. Use `roundrobin` for deterministic turn-based proposing |
| `deployment_mode` | Yes | `local` or `ansible` — determines genesis time offset and config directory |
| `config.activeEpoch` | Yes | Exponent for hash-sig active epochs (e.g., 18 means 2^18 signatures per period) |
| `config.keyType` | Yes | Always `hash-sig` for post-quantum support |
| `name` | Yes | Format: `<client>_<index>`. Client name determines which `client-cmds/*.sh` script runs |
| `privkey` | Yes | 32-byte hex string (64 chars). Used for P2P identity and ENR generation |
| `enrFields.ip` | Yes | IP address. Use `127.0.0.1` for local, real IPs for ansible |
| `enrFields.quic` | Yes | QUIC port. Must be unique per node in local mode |
| `metricsPort` | Yes | HTTP port exposed by the node. Must be unique per node in local mode. For gean and ethlambda, this maps to `--metrics-port`; the API server uses a separate `--api-port` |
| `count` | Yes | Number of validator indices. Sum of all counts = total validators |

## Default 5-Client Setup for gean Testing

```yaml
shuffle: roundrobin
deployment_mode: local

config:
  activeEpoch: 18
  keyType: "hash-sig"

validators:
  - name: "zeam_0"
    privkey: "<64-char-hex>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9001
    metricsPort: 8081
    count: 1

  - name: "ream_0"
    privkey: "<64-char-hex>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9002
    metricsPort: 8082
    count: 1

  - name: "lantern_0"
    privkey: "<64-char-hex>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9004
    metricsPort: 8084
    count: 1

  - name: "ethlambda_0"
    privkey: "<64-char-hex>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9007
    metricsPort: 8087
    count: 1

  - name: "gean_0"
    privkey: "<64-char-hex>"
    enrFields:
      ip: "127.0.0.1"
      quic: 9008
    metricsPort: 8088
    count: 1
```

This gives 5 validators across 5 nodes, one per language family
(Zig, Rust, C, Rust, Go).

## Adding a New Validator Node

1. **Choose a unique node name** following `<client>_<index>` convention:
   ```
   gean_0, gean_1, zeam_0, ream_0, lantern_0, ethlambda_0
   ```

2. **Generate a P2P private key** (64-char hex):
   ```bash
   openssl rand -hex 32
   ```

3. **Assign unique ports** (for local mode):
   - QUIC: 9001, 9002, 9003... (increment for each node)
   - Metrics/API: 8081, 8082, 8083... (increment for each node)
   - For gean and ethlambda, also assign a unique API port (5052, 5053, ...).

4. **Add the entry to `validator-config.yaml`:**
   ```yaml
   validators:
     # ... existing nodes ...

     - name: "gean_1"
       privkey: "<your-64-char-hex-key>"
       enrFields:
         ip: "127.0.0.1"        # Use real IP for ansible
         quic: 9009             # Next available port
       metricsPort: 8089        # Next available port
       count: 1
   ```

5. **Regenerate genesis with new keys:**
   ```bash
   cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --forceKeyGen
   ```

## Removing a Validator Node

1. **Delete the node entry** from `validator-config.yaml`

2. **Regenerate genesis** (required because genesis state must reflect new
   validator set):
   ```bash
   cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis
   ```
   Note: `--forceKeyGen` is NOT needed when removing. Existing keys for
   remaining indices are reused.

## Port Allocation Guide (Local Mode)

When running multiple nodes locally, each needs unique ports:

| Node | QUIC Port | Metrics Port | API Port (if applicable) |
|---|---|---|---|
| zeam_0 | 9001 | 8081 | n/a |
| ream_0 | 9002 | 8082 | n/a |
| qlean_0 | 9003 | 8083 | n/a |
| lantern_0 | 9004 | 8084 | n/a |
| lighthouse_0 | 9005 | 8085 | n/a |
| grandine_0 | 9006 | 8086 | n/a |
| ethlambda_0 | 9007 | 8087 | 5052 |
| gean_0 | 9008 | 8088 | 5058 |

When running **multiple gean or ethlambda nodes** locally, each needs a unique
`--api-port` since `validator-config.yaml` has no `apiPort` field. Pass
`--api-port` directly in `gean-cmd.sh` / `ethlambda-cmd.sh`.

For **ansible mode**, all nodes can use the same ports (9001, 8081, 5052)
since they run on different machines.

## Local vs Ansible Deployment

| Aspect | Local | Ansible |
|---|---|---|
| Config file | `lean-quickstart/local-devnet/genesis/validator-config.yaml` | `lean-quickstart/ansible-devnet/genesis/validator-config.yaml` |
| `deployment_mode` | `local` | `ansible` |
| IP addresses | `127.0.0.1` for all | Real server IPs |
| Ports | Must be unique per node | Same port, different machines |
| Genesis offset | +30 seconds | +360 seconds |
