#!/usr/bin/env bash
#
# One gean validator paired with one geth over the Engine API.
#
#   scripts/el-demo/run.sh        start geth (Docker), generate a testnet, start gean
#   scripts/el-demo/run.sh stop   stop both and remove geth's data
#
# geth runs from the ethereum/client-go image (GETH_IMAGE, default :latest,
# pulled fresh each start). Set GETH_BIN=/path/to/geth to use a local binary
# instead. Either way the execution genesis is scripts/el-demo/genesis.json:
# Shanghai and Cancun at time zero, no Prague, the EIP-4788 beacon-roots
# contract, and one funded account (the well-known test key 0xac0974...ff80).
# A fresh JWT secret is generated on every start and shared with both sides.
# geth's block 0 hash is read back over HTTP and written into the lean
# config.yaml as EXECUTION_GENESIS_BLOCK_HASH, which is what pairs the chains.
#
# On a desktop, geth's log and gean each open in their own terminal window
# (POPUP=0 keeps everything in the background with logs under DATA_DIR;
# TERMINAL=<emulator> overrides the auto-detected one). Without a display the
# background mode is used.
#
# Prerequisites: docker (or GETH_BIN), openssl, curl, python3, and `make build`
# so bin/gean and bin/keygen exist.
#
# Environment: DATA_DIR (data/el-demo), TESTNET_DIR (testnet-el-demo),
# GENESIS_DELAY (20), AUTHRPC_PORT (8551), HTTP_PORT (8545), API_PORT (5052),
# METRICS_PORT (8080), GOSSIP_PORT (9000), FEE_RECIPIENT (the funded account).

set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HERE="$REPO/scripts/el-demo"

GETH_IMAGE="${GETH_IMAGE:-ethereum/client-go:latest}"
GETH_BIN="${GETH_BIN:-}"
DATA_DIR="${DATA_DIR:-$REPO/data/el-demo}"
TESTNET_DIR="${TESTNET_DIR:-$REPO/testnet-el-demo}"
GENESIS_DELAY="${GENESIS_DELAY:-20}"
AUTHRPC_PORT="${AUTHRPC_PORT:-8551}"
HTTP_PORT="${HTTP_PORT:-8545}"
API_PORT="${API_PORT:-5052}"
METRICS_PORT="${METRICS_PORT:-8080}"
GOSSIP_PORT="${GOSSIP_PORT:-9000}"
FEE_RECIPIENT="${FEE_RECIPIENT:-0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266}"
POPUP="${POPUP:-1}"

CONTAINER="gean-el-geth"
VOLUME="gean-el-geth-data"
JWT="$DATA_DIR/jwt.hex"
PIDS="$DATA_DIR/pids"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- terminal windows -----------------------------------------------------------

# pick_terminal echoes the emulator to use for popup windows, or nothing when
# the script should stay in the background.
pick_terminal() {
  [ "$POPUP" = "1" ] || return 0
  [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ] || return 0
  if [ -n "${TERMINAL:-}" ]; then
    command -v "$TERMINAL" >/dev/null 2>&1 && echo "$TERMINAL"
    return 0
  fi
  for candidate in gnome-terminal konsole xfce4-terminal ptyxis tilix alacritty kitty x-terminal-emulator xterm; do
    if command -v "$candidate" >/dev/null 2>&1; then
      echo "$candidate"
      return 0
    fi
  done
}

# open_window TITLE COMMAND runs COMMAND in a new terminal window that stays
# open after the command exits, so the last lines remain readable.
open_window() {
  local title="$1" cmd="$2"
  local wrapped="$cmd; echo; echo '[$title exited; press enter to close]'; read -r"
  case "$TERM_EMULATOR" in
    gnome-terminal|ptyxis|tilix)
      "$TERM_EMULATOR" --title="$title" -- bash -c "$wrapped" ;;
    konsole)
      "$TERM_EMULATOR" --new-tab -p tabtitle="$title" -e bash -c "$wrapped" ;;
    xfce4-terminal)
      "$TERM_EMULATOR" --title="$title" -e "bash -c \"$wrapped\"" ;;
    alacritty)
      "$TERM_EMULATOR" --title "$title" -e bash -c "$wrapped" ;;
    kitty)
      "$TERM_EMULATOR" --title "$title" bash -c "$wrapped" ;;
    *)
      "$TERM_EMULATOR" -T "$title" -e bash -c "$wrapped" ;;
  esac >/dev/null 2>&1 &
}

# --- stop ---------------------------------------------------------------------

stop() {
  if [ -f "$PIDS" ]; then
    while read -r pid; do
      kill "$pid" 2>/dev/null || true
    done < "$PIDS"
    rm -f "$PIDS"
  fi
  # gean started in a popup window is not in the pid file; match it by its
  # config dir, which is unique to this demo.
  pkill -f -- "--custom-network-config-dir $TESTNET_DIR" 2>/dev/null || true
  if command -v docker >/dev/null 2>&1; then
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    docker volume rm "$VOLUME" >/dev/null 2>&1 || true
  fi
  log "stopped"
}

if [ "${1:-}" = "stop" ]; then
  stop
  exit 0
fi

# --- preflight ----------------------------------------------------------------

[ -x "$REPO/bin/gean" ] && [ -x "$REPO/bin/keygen" ] || die "bin/gean or bin/keygen missing; run make build"
if [ -z "$GETH_BIN" ]; then
  command -v docker >/dev/null 2>&1 || die "docker not found; install it or set GETH_BIN=/path/to/geth"
fi
TERM_EMULATOR="$(pick_terminal)"

stop 2>/dev/null || true
rm -rf "$DATA_DIR" "$TESTNET_DIR"
mkdir -p "$DATA_DIR"

log "generating a fresh JWT secret at $JWT"
openssl rand -hex 32 > "$JWT"

# --- geth ---------------------------------------------------------------------

# Inside the container geth must listen on all interfaces; the ports are
# published on loopback only, so nothing outside the host can reach them.
geth_args=(
  --networkid 32382
  --authrpc.port "$AUTHRPC_PORT" --authrpc.vhosts '*'
  --http --http.port "$HTTP_PORT" --http.api eth,net,web3 --http.vhosts '*'
  --nodiscover --maxpeers 0 --syncmode full
)

if [ -n "$GETH_BIN" ]; then
  GETH_DIR="$DATA_DIR/geth"
  mkdir -p "$GETH_DIR"
  log "initialising geth ($GETH_BIN) from $HERE/genesis.json"
  "$GETH_BIN" --datadir "$GETH_DIR" init "$HERE/genesis.json" >"$DATA_DIR/geth-init.log" 2>&1
  log "starting geth (authrpc :$AUTHRPC_PORT, http :$HTTP_PORT)"
  "$GETH_BIN" --datadir "$GETH_DIR" "${geth_args[@]}" \
    --authrpc.jwtsecret "$JWT" --authrpc.addr 127.0.0.1 --http.addr 127.0.0.1 \
    >"$DATA_DIR/geth.log" 2>&1 &
  echo $! >> "$PIDS"
  geth_log_cmd="tail -f '$DATA_DIR/geth.log'"
else
  log "pulling $GETH_IMAGE"
  docker pull "$GETH_IMAGE" >"$DATA_DIR/docker-pull.log" 2>&1 || die "docker pull failed; see $DATA_DIR/docker-pull.log"
  docker volume create "$VOLUME" >/dev/null
  log "initialising geth from $HERE/genesis.json"
  docker run --rm -v "$VOLUME:/data" -v "$HERE/genesis.json:/genesis.json:ro" \
    "$GETH_IMAGE" --datadir /data init /genesis.json >"$DATA_DIR/geth-init.log" 2>&1 \
    || die "geth init failed; see $DATA_DIR/geth-init.log"
  log "starting geth container $CONTAINER (authrpc :$AUTHRPC_PORT, http :$HTTP_PORT)"
  docker run -d --name "$CONTAINER" \
    -p "127.0.0.1:$AUTHRPC_PORT:$AUTHRPC_PORT" -p "127.0.0.1:$HTTP_PORT:$HTTP_PORT" \
    -v "$VOLUME:/data" -v "$JWT:/run/jwt.hex:ro" \
    "$GETH_IMAGE" --datadir /data "${geth_args[@]}" \
    --authrpc.jwtsecret /run/jwt.hex --authrpc.addr 0.0.0.0 --http.addr 0.0.0.0 >/dev/null
  geth_log_cmd="docker logs -f $CONTAINER"
fi

genesis_hash=""
for _ in $(seq 1 100); do
  genesis_hash="$(curl -sf -X POST "http://127.0.0.1:$HTTP_PORT" \
    -H 'content-type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"eth_getBlockByNumber","params":["0x0",false]}' \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["hash"])' 2>/dev/null || true)"
  [ -n "$genesis_hash" ] && break
  sleep 0.2
done
if [ -z "$genesis_hash" ]; then
  [ -z "$GETH_BIN" ] && docker logs "$CONTAINER" >"$DATA_DIR/geth.log" 2>&1 || true
  die "geth did not answer on :$HTTP_PORT; see $DATA_DIR/geth.log"
fi
log "execution genesis: $genesis_hash"

if [ -n "$TERM_EMULATOR" ]; then
  open_window "geth" "$geth_log_cmd"
fi

# --- lean testnet + gean ------------------------------------------------------

log "generating a one-validator lean testnet in $TESTNET_DIR"
"$REPO/bin/keygen" --validators 1 --nodes 1 --output "$TESTNET_DIR" \
  --base-port "$GOSSIP_PORT" --genesis-delay "$GENESIS_DELAY" \
  --execution-genesis-block-hash "$genesis_hash" >"$DATA_DIR/keygen.log" 2>&1

gean_cmd="'$REPO/bin/gean' \
  --custom-network-config-dir '$TESTNET_DIR' \
  --node-key '$TESTNET_DIR/node0.key' \
  --node-id node0 \
  --data-dir '$DATA_DIR/gean' \
  --is-aggregator \
  --gossipsub-port $GOSSIP_PORT \
  --api-port $API_PORT \
  --metrics-port $METRICS_PORT \
  --execution-endpoint 'http://127.0.0.1:$AUTHRPC_PORT' \
  --execution-jwt-secret '$JWT' \
  --suggested-fee-recipient '$FEE_RECIPIENT'"

log "starting gean (api :$API_PORT, metrics :$METRICS_PORT)"
if [ -n "$TERM_EMULATOR" ]; then
  # The window shows the live log and a copy lands in DATA_DIR for later.
  open_window "gean" "$gean_cmd 2>&1 | tee '$DATA_DIR/gean.log'"
  log "running in two $TERM_EMULATOR windows; logs also under $DATA_DIR"
else
  bash -c "$gean_cmd" >"$DATA_DIR/gean.log" 2>&1 &
  echo $! >> "$PIDS"
  log "running; gean log: $DATA_DIR/gean.log, geth log: $geth_log_cmd"
fi
log "stop: make node-stop"
