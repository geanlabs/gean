#!/bin/bash
set -euo pipefail

# Test the checked-out gean branch in a 5-client lean-quickstart devnet.
#
# Usage: test-branch.sh [branch-name] [--with-sync-test]
#   branch-name must be the branch HEAD is on (defaults to it).
#   RUN_DURATION=N (seconds, default 60) sets the observation window.
#
# Exit: 0 PASS, 1 FAIL (or setup error), 2 UNKNOWN (insufficient evidence).
# The devnet is left running; stop it with cleanup.sh.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GEAN_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
LEAN_QUICKSTART="${LEAN_QUICKSTART:-$GEAN_ROOT/lean-quickstart}"
GEAN_CMD="$LEAN_QUICKSTART/client-cmds/gean-cmd.sh"
STATE_DIR="${TMPDIR:-/tmp}/test-pr-devnet-$(id -u)"
NODES=(zeam_0 ream_0 lantern_0 ethlambda_0 gean_0)

die() { echo "Error: $*" >&2; exit 1; }
exists() { docker ps -aq --filter "name=^/$1\$" | grep -q .; }
running() { [[ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" == "true" ]]; }
gean_logs() { docker logs gean_0 2>&1 | sed -E 's/\x1b\[[0-9;]*m//g'; }
count_gean() { gean_logs | grep -cE "$1" || true; }
finalized_slot() {
    local slot
    slot=$(gean_logs | sed -nE 's/.*\[forkchoice\] finalized advanced slot=([0-9]+).*/\1/p' | tail -1)
    echo "${slot:-0}"
}

BRANCH_NAME=""
WITH_SYNC_TEST=false
for arg in "$@"; do
    if [[ "$arg" == "--with-sync-test" ]]; then
        WITH_SYNC_TEST=true
    elif [[ -z "$BRANCH_NAME" ]]; then
        BRANCH_NAME="$arg"
    fi
done

CURRENT=$(git -C "$GEAN_ROOT" symbolic-ref --quiet --short HEAD) \
    || die "HEAD is detached; check out the branch to test"
BRANCH_NAME="${BRANCH_NAME:-$CURRENT}"
[[ "$BRANCH_NAME" == "$CURRENT" ]] \
    || die "requested $BRANCH_NAME but HEAD is $CURRENT; run: git checkout $BRANCH_NAME"
SAFE_BRANCH=$(printf '%s' "$BRANCH_NAME" | tr -c 'A-Za-z0-9_.-' '-')
IMAGE="gean:$SAFE_BRANCH"
SHA=$(git -C "$GEAN_ROOT" rev-parse HEAD)
DIRTY=$(git -C "$GEAN_ROOT" status --porcelain)

[[ -f "$LEAN_QUICKSTART/spin-node.sh" ]] \
    || die "spin-node.sh not found in $LEAN_QUICKSTART (run 'make lean-quickstart' or set LEAN_QUICKSTART)"
[[ -f "$GEAN_CMD" ]] || die "$GEAN_CMD not found"
docker info &>/dev/null || die "docker is not running"
[[ ! -e "$STATE_DIR/containers" ]] \
    || die "a previous run is recorded in $STATE_DIR; run $SCRIPT_DIR/cleanup.sh first"
for node in "${NODES[@]}"; do
    exists "$node" && die "container $node already exists and is not owned by this run"
done

echo "=== gean devnet test ==="
echo "Branch:    $BRANCH_NAME"
echo "Commit:    $SHA"
if [[ -z "$DIRTY" ]]; then
    echo "Worktree:  clean"
else
    echo "Worktree:  DIRTY (image includes uncommitted changes)"
    echo "$DIRTY" | sed 's/^/  /'
fi
echo "Sync test: $WITH_SYNC_TEST"
echo ""

echo "[1/5] Building $IMAGE..."
docker build \
    --build-arg GIT_COMMIT="$SHA" \
    --build-arg GIT_BRANCH="$BRANCH_NAME" \
    -t "$IMAGE" \
    "$GEAN_ROOT"
IMAGE_ID=$(docker image inspect -f '{{.Id}}' "$IMAGE")
echo "Image ID:  $IMAGE_ID"
echo ""

echo "[2/5] Pointing gean-cmd.sh at $IMAGE..."
mkdir -p "$STATE_DIR"
BACKUP=$(mktemp "$STATE_DIR/gean-cmd.sh.XXXXXX")
cp "$GEAN_CMD" "$BACKUP"
SPIN_PID=""
record_owned() {
    local node
    for node in "${NODES[@]}"; do
        docker ps -aq --no-trunc --filter "name=^/$node\$"
    done > "$STATE_DIR/containers"
}
on_exit() {
    cp "$BACKUP" "$GEAN_CMD" && rm -f "$BACKUP"
    if [[ -n "$SPIN_PID" ]]; then
        record_owned
    fi
}
trap on_exit EXIT
sed -E -i "s#[^ \"]*gean:[^ \"]*#$IMAGE#" "$GEAN_CMD"
grep -q "$IMAGE" "$GEAN_CMD" || die "could not set image in $GEAN_CMD"
echo ""

echo "[3/5] Starting devnet..."
LOG="$STATE_DIR/devnet-$SAFE_BRANCH.log"
cd "$LEAN_QUICKSTART"
# --cleanData wipes per-node data dirs so each run starts from a fresh genesis.
NETWORK_DIR=local-devnet ./spin-node.sh --node all --cleanData --generateGenesis --metrics > "$LOG" 2>&1 &
SPIN_PID=$!
echo "$SPIN_PID" > "$STATE_DIR/spin.pid"
echo "spin-node.sh log: $LOG"

echo -n "Waiting for nodes"
for _ in {1..40}; do
    sleep 1
    echo -n "."
    up=0
    for node in "${NODES[@]}"; do
        running "$node" && up=$((up + 1))
    done
    [[ "$up" -eq ${#NODES[@]} ]] && break
done
echo " $up/${#NODES[@]} running"
record_owned
echo ""

RUN_DURATION="${RUN_DURATION:-60}"
if [[ "$WITH_SYNC_TEST" == "true" ]]; then
    echo "[4/5] Sync recovery test..."
    sleep 10
    FIN_START=$(finalized_slot)
    docker pause zeam_0 ream_0
    echo "Paused zeam_0 ream_0 for 20s"
    sleep 20
    docker unpause zeam_0 ream_0
    echo "Resumed; waiting 10s for sync"
    sleep 10
else
    # 4s slots: the default 60s window covers ~15 slots, enough for the
    # round-robin proposer cycle to reach gean and for finalization to move.
    echo "[4/5] Observing for ${RUN_DURATION}s..."
    sleep $((RUN_DURATION / 2))
    FIN_START=$(finalized_slot)
    sleep $((RUN_DURATION - RUN_DURATION / 2))
fi
echo ""

echo "[5/5] Results"
for node in "${NODES[@]}"; do
    if running "$node"; then echo "  $node: running"; else echo "  $node: NOT running"; fi
done
echo ""

FIN_END=$(finalized_slot)
echo "gean finalized slot: $FIN_START -> $FIN_END (during observation window)"
gean_logs | grep "Latest Finalized:" | tail -1 || true
echo "Blocks proposed: $(count_gean '\[validator\] proposed block')"
echo "ERROR lines:     $(count_gean ' ERROR ')"
if [[ "$WITH_SYNC_TEST" == "true" ]]; then
    echo "Batched fetches: $(count_gean 'batched fetch starting')"
    echo "Roots queued:    $(count_gean 'queueing missing block')"
    echo "Fetch exhausted: $(count_gean 'fetch exhausted for root')"
fi
echo ""

echo "=== Verdict ==="
STATUS=2
if ! running gean_0; then
    echo "FAIL: gean_0 is not running"
    STATUS=1
elif [[ "$(docker inspect -f '{{.Image}}' gean_0)" != "$IMAGE_ID" ]]; then
    echo "UNKNOWN: gean_0 is not running $IMAGE ($IMAGE_ID)"
elif [[ "$FIN_END" -gt "$FIN_START" ]]; then
    echo "PASS: gean finalized slot advanced $FIN_START -> $FIN_END"
    STATUS=0
elif [[ "$FIN_START" -gt 0 ]]; then
    echo "FAIL: gean finalization stalled at slot $FIN_END for the observation window"
    STATUS=1
else
    echo "UNKNOWN: gean logged no finalization; rerun with a larger RUN_DURATION or inspect peers"
fi
echo ""
echo "Logs:    docker logs gean_0 2>&1 | sed -E 's/\\x1b\\[[0-9;]*m//g' | less"
echo "Analyze: $GEAN_ROOT/.agents/skills/devnet-log-review/scripts/analyze-logs.sh"
echo "Stop:    $SCRIPT_DIR/cleanup.sh"
exit "$STATUS"
