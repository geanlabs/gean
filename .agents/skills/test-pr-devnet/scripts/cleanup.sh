#!/bin/bash
set -euo pipefail

# Stop the devnet started by test-branch.sh: remove only the containers that run
# recorded and stop its spin-node.sh. test-branch.sh restores gean-cmd.sh itself.

STATE_DIR="${TMPDIR:-/tmp}/test-pr-devnet-$(id -u)"

if [[ ! -f "$STATE_DIR/containers" ]]; then
    echo "No test-branch.sh run recorded in $STATE_DIR; nothing to clean up."
    exit 0
fi

mapfile -t IDS < "$STATE_DIR/containers"
if [[ ${#IDS[@]} -gt 0 ]]; then
    echo "Removing ${#IDS[@]} container(s) created by test-branch.sh..."
    docker rm -f "${IDS[@]}" >/dev/null
fi

if [[ -f "$STATE_DIR/spin.pid" ]]; then
    kill "$(cat "$STATE_DIR/spin.pid")" 2>/dev/null || true
fi

rm -f "$STATE_DIR/containers" "$STATE_DIR/spin.pid"
echo "Cleanup complete. Run logs remain in $STATE_DIR."
