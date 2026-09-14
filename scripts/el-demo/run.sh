#!/usr/bin/env bash
#
# One gean validator paired with one geth over the Engine API.
#
#   scripts/el-demo/run.sh        start geth, generate a testnet, start gean
#   scripts/el-demo/run.sh stop   stop both
#
# The execution genesis is scripts/el-demo/genesis.json: Shanghai and Cancun at
# time zero, no Prague, the EIP-4788 beacon-roots contract, and one funded
# account (the well-known test key 0xac0974...ff80). geth's block 0 hash is
# read back over HTTP and written into the lean config.yaml as
# EXECUTION_GENESIS_BLOCK_HASH, which is what pairs the two chains.
#
# Prerequisites: geth on PATH (or GETH=/path/to/geth), openssl, python3, and
# `make build` so bin/gean and bin/keygen exist.
#
# Environment: DATA_DIR (data/el-demo), TESTNET_DIR (testnet-el-demo),
# GENESIS_DELAY (20), AUTHRPC_PORT (8551), HTTP_PORT (8545), API_PORT (5052),
# METRICS_PORT (8080), GOSSIP_PORT (9000), FEE_RECIPIENT (the funded account).

set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HERE="$REPO/scripts/el-demo"

GETH="${GETH:-geth}"
DATA_DIR="${DATA_DIR:-$REPO/data/el-demo}"
TESTNET_DIR="${TESTNET_DIR:-$REPO/testnet-el-demo}"
GENESIS_DELAY="${GENESIS_DELAY:-20}"
AUTHRPC_PORT="${AUTHRPC_PORT:-8551}"
HTTP_PORT="${HTTP_PORT:-8545}"
API_PORT="${API_PORT:-5052}"
METRICS_PORT="${METRICS_PORT:-8080}"
GOSSIP_PORT="${GOSSIP_PORT:-9000}"
FEE_RECIPIENT="${FEE_RECIPIENT:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"

GETH_DIR="$DATA_DIR/geth"
JWT="$DATA_DIR/jwt.hex"
PIDS="$DATA_DIR/pids"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

stop() {
  if [ -f "$PIDS" ]; then
    while read -r pid; do
      kill "$pid" 2>/dev/null || true
    done < "$PIDS"
    rm -f "$PIDS"
  fi
  log "stopped"
}

if [ "${1:-}" = "stop" ]; then
  stop
  exit 0
fi

command -v "$GETH" >/dev/null 2>&1 || die "geth not found; install it or set GETH=/path/to/geth"
[ -x "$REPO/bin/gean" ] && [ -x "$REPO/bin/keygen" ] || die "bin/gean or bin/keygen missing; run make build"

stop 2>/dev/null || true
rm -rf "$DATA_DIR" "$TESTNET_DIR"
mkdir -p "$GETH_DIR"
openssl rand -hex 32 > "$JWT"

log "initialising geth from $HERE/genesis.json"
"$GETH" --datadir "$GETH_DIR" init "$HERE/genesis.json" >"$DATA_DIR/geth-init.log" 2>&1

log "starting geth (authrpc :$AUTHRPC_PORT, http :$HTTP_PORT)"
"$GETH" --datadir "$GETH_DIR" \
  --networkid 32382 \
  --authrpc.addr 127.0.0.1 --authrpc.port "$AUTHRPC_PORT" --authrpc.jwtsecret "$JWT" \
  --http --http.addr 127.0.0.1 --http.port "$HTTP_PORT" --http.api eth,net,web3 \
  --nodiscover --maxpeers 0 --syncmode full \
  >"$DATA_DIR/geth.log" 2>&1 &
echo $! >> "$PIDS"

genesis_hash=""
for _ in $(seq 1 50); do
  genesis_hash="$(curl -sf -X POST "http://127.0.0.1:$HTTP_PORT" \
    -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber","params":["0x0",false]}' \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["hash"])' 2>/dev/null || true)"
  [ -n "$genesis_hash" ] && break
  sleep 0.2
done
[ -n "$genesis_hash" ] || die "geth did not answer on :$HTTP_PORT; see $DATA_DIR/geth.log"
log "execution genesis: $genesis_hash"

log "generating a one-validator lean testnet in $TESTNET_DIR"
"$REPO/bin/keygen" --validators 1 --nodes 1 --output "$TESTNET_DIR" \
  --base-port "$GOSSIP_PORT" --genesis-delay "$GENESIS_DELAY" \
  --execution-genesis-block-hash "$genesis_hash" >"$DATA_DIR/keygen.log" 2>&1

log "starting gean (api :$API_PORT, metrics :$METRICS_PORT)"
"$REPO/bin/gean" \
  --custom-network-config-dir "$TESTNET_DIR" \
  --node-key "$TESTNET_DIR/node0.key" \
  --node-id node0 \
  --data-dir "$DATA_DIR/gean" \
  --is-aggregator \
  --gossipsub-port "$GOSSIP_PORT" \
  --api-port "$API_PORT" \
  --metrics-port "$METRICS_PORT" \
  --execution-endpoint "http://127.0.0.1:$AUTHRPC_PORT" \
  --execution-jwt-secret "$JWT" \
  --suggested-fee-recipient "$FEE_RECIPIENT" \
  >"$DATA_DIR/gean.log" 2>&1 &
echo $! >> "$PIDS"

log "running; logs in $DATA_DIR/{geth,gean}.log"
log "watch:  tail -f $DATA_DIR/gean.log | grep -E 'execution|proposed block'"
log "stop:   scripts/el-demo/run.sh stop"
