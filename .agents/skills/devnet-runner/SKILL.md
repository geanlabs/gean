---
name: devnet-runner
description: Manage local development networks (devnets) for lean consensus multi-client testing. This skill should be used when the user asks to run a devnet, start or stop devnet nodes, spin up a local testnet, configure validator nodes, regenerate genesis files, change Docker image tags, collect or dump node logs, troubleshoot devnet issues, restart a node with checkpoint sync, run a long-lived devnet with detached containers, or perform rolling restarts to upgrade images.
---

# Devnet Runner

Manage local development networks for lean consensus testing involving gean
and peer clients (zeam, ream, lantern, ethlambda).

## Where to Run

Devnets run on the user's devnet server, never on the local laptop. If the host
is unknown, ask for it before running anything. All commands below run in the
gean checkout on that server; the scripts are host-agnostic.

## Prerequisites

`make lean-quickstart` clones lean-quickstart into `lean-quickstart/` inside
the gean repo root. The paths below are relative to the gean repo root.

## Default Behavior

When starting a devnet, **always**:
1. **Update validator config** — Edit
   `lean-quickstart/local-devnet/genesis/validator-config.yaml` to include
   ONLY the nodes that will run. Remove entries for nodes that won't be
   started (unless the user explicitly asks to keep them). Validator indices
   are assigned to ALL nodes in the config; if a node is in the config but
   not running, its validators will miss their proposer slots. To control
   which nodes run, always edit this config file rather than using
   `--node <specific>`, since `--node` does NOT reassign validators and
   causes missed slots.
2. **Update client image tags** — If the user specifies a tag (e.g., "use
   `dev` tag for gean"), edit the relevant
   `lean-quickstart/client-cmds/{client}-cmd.sh` file to update the
   `node_docker` image tag.
3. **Use run-devnet-with-timeout.sh** — This script runs all nodes in the
   config with a timeout, dumps logs, then stops them.
4. Run for **20 slots** unless the user specifies otherwise.
5. The script dumps each configured node's log to `<node_name>.log` in the
   gean repo root, then stops spin-node.sh and removes only the containers it
   created. It refuses to start if a container already uses a configured node
   name (another devnet may own it).

## Timing Calculation

Total timeout = startup buffer + genesis offset + (slots × 4 seconds)

| Component | Local Mode | Ansible Mode |
|---|---|---|
| Startup buffer | 10s | 10s |
| Genesis offset | 30s | 360s |
| Per slot | 4s | 4s |

**Examples (local mode):**
- 20 slots: 10 + 30 + (20 × 4) = **120s**
- 50 slots: 10 + 30 + (50 × 4) = **240s**
- 100 slots: 10 + 30 + (100 × 4) = **440s**

## Quick Start (Default Workflow)

**Step 1: Configure nodes** — Edit
`lean-quickstart/local-devnet/genesis/validator-config.yaml` to keep only the
nodes you want to run. See `references/validator-config.md` for the full
schema and field reference.

**Step 2: Update image tags (if needed)** — Edit
`lean-quickstart/client-cmds/{client}-cmd.sh` to change the Docker image tag
in `node_docker`. See `references/clients.md` for current default tags.

**Step 3: Run the devnet**
```bash
# Start devnet with fresh genesis, capture logs directly (20 slots = 120s)
.agents/skills/devnet-runner/scripts/run-devnet-with-timeout.sh 120
```

## Manual Commands

All `spin-node.sh` commands must be run from within `lean-quickstart/`:

```bash
# Stop all nodes
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --stop

# Run for custom duration (e.g., 50 slots = 240s with genesis offset)
.agents/skills/devnet-runner/scripts/run-devnet-with-timeout.sh 240

# Start without timeout (press Ctrl+C to stop)
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis
```

## Command-Line Flags

| Flag | Description |
|---|---|
| `--node <name|all>` | **Required.** Node(s) to start. Use `all` to start all nodes in config |
| `--generateGenesis` | Regenerate genesis files. Implies `--cleanData` |
| `--cleanData` | Clean data directories before starting |
| `--stop` | Stop running nodes instead of starting them |
| `--forceKeyGen` | Force regeneration of hash-sig validator keys |
| `--validatorConfig <path>` | Custom config path (default: `$NETWORK_DIR/genesis/validator-config.yaml`) |
| `--dockerWithSudo` | Run docker commands with `sudo` |

## Changing Docker Image Tags

To use a specific tag for certain clients, edit the
`lean-quickstart/client-cmds/{client}-cmd.sh` files before running.

**Example:** point gean at a locally built image. `make docker-build` tags
`gean:$(VERSION)` (`git describe --tags --always --dirty`) and
`ghcr.io/geanlabs/gean:devnet5`. Replace the image reference in `node_docker`
in `lean-quickstart/client-cmds/gean-cmd.sh` with one of those tags.

See `references/clients.md` for the default images.

## gean-Specific Notes

- gean's flag defaults (`cmd/gean/flags.go`) are `--gossipsub-port 9000`,
  `--api-port 5052`, `--metrics-port 5054`. The Makefile's `make run`,
  `run-node1`, `run-node2` use 9000/5052/8080, 9001/5053/8081, 9002/5054/8082.
  In a devnet, ports come from `validator-config.yaml` and
  `lean-quickstart/client-cmds/gean-cmd.sh`.
- gean expects `--custom-network-config-dir`, `--node-key`, `--node-id`,
  `--data-dir`, `--gossipsub-port`, `--api-port`, `--metrics-port`, and
  optionally `--is-aggregator` and `--checkpoint-sync-url`.
- To configure gean as the aggregator in a devnet, ensure
  `--is-aggregator` is set in `gean-cmd.sh` for exactly one gean instance.

## Validator Configuration

See `references/validator-config.md` for the full schema, field reference,
adding/removing nodes, port allocation guide, and local vs ansible deployment
differences.

## Log Collection

### View Live Logs
```bash
docker logs gean_0           # View current logs
docker logs -f gean_0        # Follow/stream logs
```

### Dump Logs to Files

**Automatic:** When using `run-devnet-with-timeout.sh`, logs are automatically
dumped to `<node_name>.log` files in the gean repo root before stopping.

**Single node (manual):**
```bash
docker logs gean_0 &> gean_0.log
```

**All running nodes (manual):**
```bash
for node in $(docker ps --format '{{.Names}}' | grep -E '^(gean|zeam|ream|lantern|ethlambda)_'); do
  docker logs "$node" &> "${node}.log"
done
```

### Data Directory Logs

Client-specific data and file-based logs are stored at:
```
lean-quickstart/local-devnet/data/<node_name>/
```

## Common Troubleshooting

### Nodes Won't Start

1. Check if containers are already running:
   ```bash
   docker ps | grep -E 'gean|zeam|ream|lantern|ethlambda'
   ```
2. Stop existing nodes first:
   ```bash
   cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --stop
   ```

### Nodes Not Finding Peers

1. Verify all nodes are using the same genesis:
   ```bash
   cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis
   ```
2. Check `nodes.yaml` was generated with correct ENR records

### Genesis Mismatch Errors

Regenerate genesis for all nodes:
```bash
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --forceKeyGen
```

### Port Conflicts

Check whether the ports configured for the node in `validator-config.yaml`
and `gean-cmd.sh` are in use, e.g. `lsof -i :<port>`.

### Stale Containers Cause Genesis Mismatch

If you see `UnknownSourceBlock` or `OutOfMemory` deserialization errors, a
container from a previous run may still be running with old genesis.

**Fix:** check `docker ps -a` and remove only containers that belong to your
devnet; the server may host other devnets using the same names.
`run-devnet-with-timeout.sh` refuses to start when a configured name is taken
and removes only what it created.

### Docker Permission Issues

```bash
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --dockerWithSudo
```

## Scripts

| Script | Description |
|---|---|
| `scripts/run-devnet-with-timeout.sh <seconds>` | Run devnet for specified duration, dump logs to gean repo root, then remove the containers it created |

## Long-Lived Devnets and Rolling Restarts

For persistent devnets on the user's devnet server, use
detached containers instead of `spin-node.sh`. This allows rolling restarts to
upgrade images without losing chain state.

**Key points:**
- Start containers with `docker run -d --restart unless-stopped` (not
  `spin-node.sh`)
- Rolling restart: stop one node, **wait 60 seconds** (gossipsub backoff),
  start with new image + checkpoint sync
- Restart non-aggregator nodes first, aggregator last
- Checkpoint sync URL uses gean's API port:
  `http://127.0.0.1:<api-port>/lean/v0/states/finalized`

See `references/long-lived-devnet.md` for the full procedure.

## Reference

- `references/clients.md`: Canonical client reference (images, ports, build)
- `references/validator-config.md`: Full config schema, field reference, adding/removing nodes, port allocation
- `references/long-lived-devnet.md`: Persistent devnets with detached containers and rolling restarts
