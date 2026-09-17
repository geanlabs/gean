#!/usr/bin/env bash
# One gean validator paired with geth in Docker over the Engine API; logs under
# DATA_DIR. Requires make build. Usage: scripts/el-demo/run-remote.sh [stop]
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HERE="$REPO/scripts/el-demo"
GETH_IMAGE="${GETH_IMAGE:-ethereum/client-go:latest}"
DATA_DIR="${DATA_DIR:-$REPO/data/el-demo-remote}"
TESTNET_DIR="${TESTNET_DIR:-$REPO/testnet-el-demo-remote}"
GENESIS_DELAY="${GENESIS_DELAY:-20}"
AUTHRPC_PORT="${AUTHRPC_PORT:-8551}"
HTTP_PORT="${HTTP_PORT:-8545}"
API_PORT="${API_PORT:-5052}"
METRICS_PORT="${METRICS_PORT:-8080}"
GOSSIP_PORT="${GOSSIP_PORT:-9000}"
FEE_RECIPIENT="${FEE_RECIPIENT:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"
CONTAINER="gean-el-geth"
VOLUME="gean-el-geth-data"
JWT="$DATA_DIR/jwt.hex"
PIDS="$DATA_DIR/pids"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

stop() {
  if [ -f "$PIDS" ]; then
    while read -r pid; do kill "$pid" 2>/dev/null || true; done < "$PIDS"
    rm -f "$PIDS"
  fi
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  docker volume rm "$VOLUME" >/dev/null 2>&1 || true
  log "stopped"
}

if [ "${1:-}" = "stop" ]; then stop; exit 0; fi
[ -x "$REPO/bin/gean" ] && [ -x "$REPO/bin/keygen" ] || die "run make build first"
for tool in docker openssl curl python3; do
  command -v "$tool" >/dev/null || die "$tool not found"
done
stop
rm -rf "$DATA_DIR" "$TESTNET_DIR"
mkdir -p "$DATA_DIR"
openssl rand -hex 32 > "$JWT"

log "pulling and initialising $GETH_IMAGE"
docker pull "$GETH_IMAGE" >"$DATA_DIR/docker-pull.log" 2>&1
docker volume create "$VOLUME" >/dev/null
docker run --rm -v "$VOLUME:/data" -v "$HERE/genesis.json:/genesis.json:ro" \
  "$GETH_IMAGE" --datadir /data init /genesis.json >"$DATA_DIR/geth-init.log" 2>&1
# Publish both APIs on loopback only.
docker run -d --name "$CONTAINER" \
  -p "127.0.0.1:$AUTHRPC_PORT:$AUTHRPC_PORT" -p "127.0.0.1:$HTTP_PORT:$HTTP_PORT" \
  -v "$VOLUME:/data" -v "$JWT:/run/jwt.hex:ro" \
  "$GETH_IMAGE" --datadir /data --networkid 32382 \
  --authrpc.port "$AUTHRPC_PORT" --authrpc.addr 0.0.0.0 --authrpc.vhosts '*' \
  --authrpc.jwtsecret /run/jwt.hex \
  --http --http.port "$HTTP_PORT" --http.addr 0.0.0.0 --http.vhosts '*' --http.api eth,net,web3 \
  --nodiscover --maxpeers 0 --syncmode full >/dev/null
docker logs -f "$CONTAINER" >"$DATA_DIR/geth.log" 2>&1 &
echo $! >> "$PIDS"

genesis_hash=""
for ((attempt=0; attempt<100; attempt++)); do
  genesis_hash="$(curl -sf --max-time 1 "http://127.0.0.1:$HTTP_PORT" \
    -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber","params":["0x0",false]}' \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["hash"])' 2>/dev/null || true)"
  [ -n "$genesis_hash" ] && break
  sleep 0.2
done
[ -n "$genesis_hash" ] || die "geth did not answer on :$HTTP_PORT; see $DATA_DIR/geth.log"
log "execution genesis: $genesis_hash"

"$REPO/bin/keygen" --validators 1 --nodes 1 --output "$TESTNET_DIR" \
  --base-port "$GOSSIP_PORT" --genesis-delay "$GENESIS_DELAY" \
  --execution-genesis-block-hash "$genesis_hash" >"$DATA_DIR/keygen.log" 2>&1
"$REPO/bin/gean" \
  --custom-network-config-dir "$TESTNET_DIR" --node-key "$TESTNET_DIR/node0.key" \
  --node-id node0 --data-dir "$DATA_DIR/gean" --is-aggregator \
  --gossipsub-port "$GOSSIP_PORT" --api-port "$API_PORT" --metrics-port "$METRICS_PORT" \
  --execution-endpoint "http://127.0.0.1:$AUTHRPC_PORT" \
  --execution-jwt-secret "$JWT" --suggested-fee-recipient "$FEE_RECIPIENT" \
  >"$DATA_DIR/gean.log" 2>&1 &
echo $! >> "$PIDS"
log "running; logs: $DATA_DIR/{gean,geth}.log; stop: make node-stop"
