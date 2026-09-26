#!/bin/bash
set -euo pipefail

# Status of the devnet started by test-branch.sh (only containers it recorded).

STATE_DIR="${TMPDIR:-/tmp}/test-pr-devnet-$(id -u)"
strip() { sed -E 's/\x1b\[[0-9;]*m//g'; }

if [[ ! -s "$STATE_DIR/containers" ]]; then
    echo "No test-branch.sh run recorded in $STATE_DIR." >&2
    exit 1
fi

while read -r id; do
    name=$(docker inspect -f '{{.Name}}' "$id" 2>/dev/null | tr -d /) || { echo "$id: gone"; continue; }
    state=$(docker inspect -f '{{.State.Status}}' "$id")
    logs=$(docker logs --tail 300 "$id" 2>&1 | strip)
    errors=$(docker logs "$id" 2>&1 | strip | grep -cw ERROR || true)
    echo "$name: $state, ERROR lines: $errors"
    if [[ "$name" == gean_0 ]]; then
        grep "CHAIN STATUS" <<< "$logs" | tail -1 | sed 's/^/  /' || true
        grep "Latest Finalized:" <<< "$logs" | tail -1 | sed 's/^/  /' || true
        grep "\[forkchoice\] finalized advanced" <<< "$logs" | tail -1 | sed 's/^/  /' || true
    else
        grep -i "finalized" <<< "$logs" | tail -1 | sed 's/^/  /' || true
    fi
done < "$STATE_DIR/containers"
