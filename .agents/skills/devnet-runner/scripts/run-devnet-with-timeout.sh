#!/bin/bash
# Run the lean-quickstart devnet for <seconds>, dump node logs to the gean repo
# root, then stop spin-node.sh and remove only the containers this run created.
#
# Refuses to start if a container already uses a node name from
# validator-config.yaml, so every container with those names is owned by this run.
#
# Usage: run-devnet-with-timeout.sh <seconds>
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <seconds>" >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
QUICKSTART_DIR="$REPO_ROOT/lean-quickstart"
CONFIG="$QUICKSTART_DIR/local-devnet/genesis/validator-config.yaml"

if [[ ! -f "$CONFIG" ]]; then
    echo "Error: $CONFIG not found. Run 'make lean-quickstart' in $REPO_ROOT first." >&2
    exit 1
fi

mapfile -t NODES < <(sed -nE 's/^[[:space:]]*-[[:space:]]*name:[[:space:]]*"?([^"#[:space:]]+)"?.*/\1/p' "$CONFIG")
if [[ ${#NODES[@]} -eq 0 ]]; then
    echo "Error: no node names found in $CONFIG" >&2
    exit 1
fi

for node in "${NODES[@]}"; do
    if docker ps -aq --filter "name=^/${node}\$" | grep -q .; then
        echo "Error: container $node already exists and is not owned by this run." >&2
        echo "Stop it deliberately (it may belong to another devnet) or rename nodes in $CONFIG." >&2
        exit 1
    fi
done

owned_containers() {
    local node
    for node in "${NODES[@]}"; do
        docker ps -aq --filter "name=^/${node}\$"
    done
}

SPIN_PID=""
cleanup() {
    local ids
    ids=$(owned_containers)
    echo "Removing containers created by this run..."
    [[ -n "$ids" ]] && docker rm -f $ids >/dev/null
    if [[ -n "$SPIN_PID" ]]; then
        kill "$SPIN_PID" 2>/dev/null || true
        wait "$SPIN_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

cd "$QUICKSTART_DIR"
# --cleanData wipes per-node data dirs so each run starts from a fresh genesis.
# Process substitution keeps $! pointing at spin-node.sh rather than tee.
NETWORK_DIR=local-devnet ./spin-node.sh --node all --cleanData --generateGenesis \
    > >(tee "$REPO_ROOT/devnet.log") 2>&1 &
SPIN_PID=$!
sleep "$1"

echo "Dumping node logs..."
for node in "${NODES[@]}"; do
    if docker ps -aq --filter "name=^/${node}\$" | grep -q .; then
        docker logs "$node" > "$REPO_ROOT/${node}.log" 2>&1
        echo "  Dumped ${node}.log"
    else
        echo "  $node: no container (did it start?)"
    fi
done
