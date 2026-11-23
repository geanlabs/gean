#!/bin/bash
# Run a local lean devnet with this gean build for a number of seconds, dump every node's log to
# the repository root, then stop the nodes.
#
# Usage: run-devnet-with-timeout.sh <seconds>

set -uo pipefail

if [ -z "${1:-}" ]; then
    echo "Usage: $0 <seconds>"
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
NODES='^gean_'

# Build the image and clone lean-quickstart first, so the timeout covers only the devnet.
make -C "$REPO_ROOT" docker-build lean-quickstart || exit 1

# make lean-devnet points lean-quickstart at the image just built (a cached rebuild) and starts
# every node in validator-config.yaml with fresh genesis; its output goes to devnet.log. It runs in
# its own process group so it can be stopped as a whole.
setsid make -C "$REPO_ROOT" lean-devnet &
PGID=$!
sleep "$1"

echo "Dumping node logs..."
for node in $(docker ps --format '{{.Names}}' | grep -E "$NODES"); do
    docker logs "$node" > "$REPO_ROOT/${node}.log" 2>&1
    echo "  ${node}.log"
done

kill -- -"$PGID" 2>/dev/null
wait "$PGID" 2>/dev/null

# spin-node.sh may leave containers behind when killed.
echo "Removing containers..."
for node in $(docker ps -a --format '{{.Names}}' | grep -E "$NODES"); do
    docker rm -f "$node" > /dev/null 2>&1
done
