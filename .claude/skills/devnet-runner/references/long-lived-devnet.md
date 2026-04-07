# Long-Lived Devnets

Running a persistent devnet with detached containers that survive SSH
disconnects and support rolling restarts to upgrade images without losing
chain state.

## When to Use

- Running a devnet on a remote server that should persist across SSH sessions
- Upgrading gean images mid-devnet without resetting genesis
- Testing checkpoint sync and rolling restart procedures

## Overview

`spin-node.sh` runs containers with `docker run --rm` (foreground, auto-remove)
and kills all containers on exit. This is fine for short test runs but not for
long-lived devnets.

The alternative: start containers directly with
`docker run -d --restart unless-stopped`. Containers are decoupled from any
parent process and survive SSH disconnects, script exits, and host reboots.

## Starting a Long-Lived Devnet

### Step 1: Generate genesis

Use `spin-node.sh` to generate genesis config, keys, and ENR records, then
immediately stop it:

```bash
cd lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis
# Press Ctrl-C after nodes start (genesis is already generated)
```

Or update `GENESIS_TIME` in `config.yaml` manually:

```bash
GENESIS=/path/to/lean-quickstart/local-devnet/genesis
GENESIS_TIME=$(($(date +%s) + 30))
sed -i "s/^GENESIS_TIME:.*/GENESIS_TIME: $GENESIS_TIME/" $GENESIS/config.yaml
```

### Step 2: Start all containers detached

Start all nodes simultaneously so the gossipsub mesh forms correctly. Example
for a 5-client setup (zeam, ream, lantern, ethlambda, gean):

```bash
GENESIS=/path/to/lean-quickstart/local-devnet/genesis
DATA=/path/to/lean-quickstart/local-devnet/data
GEAN_IMAGE=gean:dev
ZEAM_IMAGE=blockblaz/zeam:devnet1
REAM_IMAGE=ghcr.io/reamlabs/ream:latest
LANTERN_IMAGE=piertwo/lantern:v0.0.1
ETHLAMBDA_IMAGE=ghcr.io/lambdaclass/ethlambda:devnet3

# Clean data dirs
for d in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
  rm -rf "$DATA/$d/"*
done

# zeam
docker run -d --restart unless-stopped --name zeam_0 --network host \
  -v $GENESIS:/config -v $DATA/zeam_0:/data \
  $ZEAM_IMAGE node \
  --custom-network-config-dir /config \
  --gossipsub-port 9001 --node-id zeam_0 \
  --node-key /config/zeam_0.key \
  --metrics-port 8081

# ream
docker run -d --restart unless-stopped --name ream_0 --network host \
  -v $GENESIS:/config -v $DATA/ream_0:/data \
  $REAM_IMAGE \
  --custom-network-config-dir /config \
  --gossipsub-port 9002 --node-id ream_0 \
  --node-key /config/ream_0.key \
  --metrics-port 8082

# lantern
docker run -d --restart unless-stopped --name lantern_0 --network host \
  -v $GENESIS:/config -v $DATA/lantern_0:/data \
  $LANTERN_IMAGE \
  --custom-network-config-dir /config \
  --gossipsub-port 9004 --node-id lantern_0 \
  --node-key /config/lantern_0.key \
  --metrics-port 8084

# ethlambda
docker run -d --restart unless-stopped --name ethlambda_0 --network host \
  -v $GENESIS:/config -v $DATA/ethlambda_0:/data \
  $ETHLAMBDA_IMAGE \
  --custom-network-config-dir /config \
  --gossipsub-port 9007 --node-id ethlambda_0 \
  --node-key /config/ethlambda_0.key \
  --http-address 0.0.0.0 --api-port 5052 --metrics-port 8087

# gean (aggregator)
docker run -d --restart unless-stopped --name gean_0 --network host \
  -v $GENESIS:/config -v $DATA/gean_0:/data \
  $GEAN_IMAGE \
  --custom-network-config-dir /config \
  --gossipsub-port 9008 --node-id gean_0 \
  --node-key /config/gean_0.key \
  --is-aggregator \
  --http-address 0.0.0.0 --api-port 5058 --metrics-port 8088
```

Do NOT include `--checkpoint-sync-url` in the initial start. Nodes start from
genesis.

### Step 3: Verify

Wait ~50 seconds (30s genesis offset + 20s for finalization to start), then
check:

```bash
for n in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
  printf "$n: "
  docker logs --tail 30 "$n" 2>&1 | grep -i "finalized\|finalized_slot" | tail -1
done
```

All nodes should show the same finalized slot advancing.

## Rolling Restart Procedure (gean)

To upgrade gean's image without losing chain state. Restart one node at a time;
the network continues finalizing with the remaining nodes.

### Critical: 60-Second Wait

After stopping a node, **wait at least 60 seconds** before starting the
replacement. This allows the gossipsub backoff timer on other nodes to expire.
Without this wait, the restarted node's GRAFT requests are rejected and it
never joins the gossip mesh, meaning it won't receive blocks or attestations
via gossip.

### Restart Order

1. Non-aggregator nodes first
2. Aggregator (gean_0) last (while it's offline, gean's aggregations stop and
   finalization stalls)

### Per-Node Procedure

For each gean node:

```bash
GENESIS=/path/to/lean-quickstart/local-devnet/genesis
DATA=/path/to/lean-quickstart/local-devnet/data
NEW_IMAGE=gean:my-new-tag

# 1. Pull or build the new image first (minimizes downtime)
docker pull $NEW_IMAGE  # if remote
# OR
make docker-build       # if building locally

# 2. Pick a healthy peer's API port as checkpoint source
#    (any running gean or ethlambda node that is NOT the one being restarted)
#    gean serves /lean/v0/states/finalized on --api-port
CHECKPOINT_SOURCE_PORT=5052  # ethlambda_0's API port (or another gean's port)

# 3. Stop and remove the container
docker rm -f gean_0
rm -rf "$DATA/gean_0/"*

# 4. Wait 60 seconds for gossipsub backoff to expire
sleep 60

# 5. Start with new image + checkpoint sync
docker run -d --restart unless-stopped --name gean_0 --network host \
  -v $GENESIS:/config -v $DATA/gean_0:/data \
  $NEW_IMAGE \
  --custom-network-config-dir /config \
  --gossipsub-port 9008 --node-id gean_0 \
  --node-key /config/gean_0.key \
  --is-aggregator \
  --http-address 0.0.0.0 --api-port 5058 --metrics-port 8088 \
  --checkpoint-sync-url http://127.0.0.1:$CHECKPOINT_SOURCE_PORT/lean/v0/states/finalized
```

### Verification After Each Node

Wait ~20 seconds, then verify:

```bash
# Check the restarted node receives blocks via gossip (not just req-resp)
docker logs --tail 30 gean_0 2>&1 | grep -i "received block\|imported block"

# Check finalization matches other nodes
for n in zeam_0 ream_0 lantern_0 ethlambda_0 gean_0; do
  printf "$n: "
  docker logs --tail 30 "$n" 2>&1 | grep -i "finalized" | tail -1
done
```

**Only proceed to the next node after confirming:**
- The restarted node shows incoming gossip blocks
- No "NoPeersSubscribedToTopic" warnings in recent logs
- Finalized slot matches other nodes

## Monitoring Stack

If Prometheus and Grafana were previously started via `spin-node.sh --metrics`,
restart them separately since they're managed by docker-compose:

```bash
cd lean-quickstart/metrics && docker compose -f docker-compose-metrics.yaml up -d
```

## Troubleshooting

### Restarted node shows "NoPeersSubscribedToTopic" persistently

The 60-second wait was not long enough, or was skipped. Stop the node, wait
60s, and start again.

### Finalization stalls after restarting the aggregator

Expected behavior. Finalization resumes once the aggregator catches up to head
and starts aggregating attestations again. This typically takes 10-20 seconds
after the node starts.

### Chain doesn't progress after restarting all nodes

If all nodes were restarted from genesis (no checkpoint sync) with a stale
`GENESIS_TIME`, the slot gap from genesis to current time may not satisfy
3SF-mini justifiability rules. Regenerate genesis with a fresh timestamp.

### "genesis time mismatch" or "validator count mismatch"

The checkpoint source is running a different genesis than the restarting node.
Ensure both use the same genesis config directory (`-v $GENESIS:/config`).

### "HTTP request failed" or connection refused

The checkpoint source node is down or unreachable. Verify with `curl`:
```bash
curl -s http://127.0.0.1:<api-port>/lean/v0/health
# Should return: {"status":"healthy",...}
```

### Container name conflict on start

The old container wasn't fully removed. Use `docker rm -f <name>` before
`docker run`.

### "Fallback pruning (finalization stalled)" after catch-up

Normal during catch-up. The node accumulated blocks faster than finalization
can advance. Resolves once fully caught up.

### gean checkpoint sync gives wrong finalized slot

This was a bug in `cmd/gean/main.go` that mixed the served block's root with
the served state's internal `LatestFinalized.Slot`. **Fixed in commit
e7e752c.** If you see this on an older gean image, upgrade to >= e7e752c.
