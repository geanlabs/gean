#!/usr/bin/env bash
# One gean validator with geth embedded in the same process. No Docker, no
# JWT, one binary. Requires make build. Usage: scripts/el-demo/run.sh [stop]
#
# NODES=n runs n validators as n gean processes, each with its own embedded
# geth, the geths peered through node 0. Keys are generated once per
# TESTNET_DIR and reused; genesis time is refreshed on every start.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GENESIS="$REPO/scripts/el-demo/genesis.json"
NODES="${NODES:-1}"
DATA_DIR="${DATA_DIR:-$REPO/data/el-demo}"
TESTNET_DIR="${TESTNET_DIR:-$REPO/testnet-el-demo}"
GENESIS_DELAY="${GENESIS_DELAY:-20}"
GOSSIP_PORT="${GOSSIP_PORT:-9000}"
API_PORT="${API_PORT:-5052}"
METRICS_PORT="${METRICS_PORT:-8080}"
EL_HTTP_PORT="${EL_HTTP_PORT:-8545}"
EL_P2P_PORT="${EL_P2P_PORT:-30303}"
FEE_RECIPIENT="${FEE_RECIPIENT:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"
PIDS="$DATA_DIR/pids"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

stop() {
  if [ -f "$PIDS" ]; then
    while read -r pid; do kill "$pid" 2>/dev/null || true; done < "$PIDS"
    rm -f "$PIDS"
  fi
  log "stopped"
}

if [ "${1:-}" = "stop" ]; then stop; exit 0; fi
[ -x "$REPO/bin/gean" ] && [ -x "$REPO/bin/keygen" ] || die "run make build first"
stop
rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"

log "generating a $NODES-validator lean testnet in $TESTNET_DIR (keys are reused if present)"
"$REPO/bin/keygen" --validators "$NODES" --nodes "$NODES" --output "$TESTNET_DIR" \
  --base-port "$GOSSIP_PORT" --genesis-delay "$GENESIS_DELAY" \
  --execution-genesis-block-hash "$GENESIS" >"$DATA_DIR/keygen.log" 2>&1

bootnodes=""
for ((i=0; i<NODES; i++)); do
  args=(
    --custom-network-config-dir "$TESTNET_DIR" --node-key "$TESTNET_DIR/node$i.key"
    --node-id "node$i" --data-dir "$DATA_DIR/node$i"
    --gossipsub-port $((GOSSIP_PORT + i)) --api-port $((API_PORT + i)) --metrics-port $((METRICS_PORT + i))
    --el-genesis "$GENESIS" --el-http-port $((EL_HTTP_PORT + i)) --el-p2p-port $((EL_P2P_PORT + i))
    --suggested-fee-recipient "$FEE_RECIPIENT"
  )
  [ "$i" -eq 0 ] && args+=(--is-aggregator)
  [ -n "$bootnodes" ] && args+=(--el-bootnodes "$bootnodes")
  log "starting node$i (api :$((API_PORT + i)), eth rpc :$((EL_HTTP_PORT + i)))"
  "$REPO/bin/gean" "${args[@]}" >"$DATA_DIR/node$i.log" 2>&1 &
  echo $! >> "$PIDS"
  if [ "$i" -eq 0 ] && [ "$NODES" -gt 1 ]; then
    # The other execution clients peer through node 0's enode.
    for _ in $(seq 1 300); do
      bootnodes="$(grep -o 'enode://[^ ]*' "$DATA_DIR/node0.log" | head -1 || true)"
      [ -n "$bootnodes" ] && break
      sleep 0.2
    done
    [ -n "$bootnodes" ] || die "node0 did not announce its execution enode; see $DATA_DIR/node0.log"
  fi
done
log "running; logs: $DATA_DIR/node*.log; stop: make node-stop"
