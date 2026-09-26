#!/bin/bash
# check-consensus-progress.sh - Show consensus progress per node
#
# Usage: check-consensus-progress.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: head, justified, and finalized slot from each node's last gean
# fork-choice head event ("[forkchoice] head slot=" or "[forkchoice] REORG"),
# then the slots each gean node proposed. N/A means no recognized event; only
# gean's log patterns are verified.

set -euo pipefail

log_dir="${1:-.}"

strip_ansi() {
    sed -E 's/\x1b\[[0-9;]*m//g'
}

shopt -s nullglob
log_files=()
for f in "$log_dir"/*.log; do
    [[ "$(basename "$f")" == "devnet.log" ]] || log_files+=("$f")
done
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No node .log files found in $log_dir" >&2
    exit 1
fi

echo "=== Consensus Progress (Last Observed Head Event) ==="
printf "%-20s %12s %12s %12s\n" "Node" "Head Slot" "Justified" "Finalized"
printf "%-20s %12s %12s %12s\n" "----" "---------" "---------" "---------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    progress=$(strip_ansi < "$f" \
        | grep -E '\[forkchoice\] (head|REORG depth=[0-9]+) slot=[0-9]+' \
        | tail -1 \
        | sed -E 's/.* slot=([0-9]+) .* justified_slot=([0-9]+) .* finalized_slot=([0-9]+) .*/\1 \2 \3/' || true)
    read -r head justified finalized <<< "${progress:-N/A N/A N/A}"
    printf "%-20s %12s %12s %12s\n" "$node" "$head" "$justified" "$finalized"
done

echo ""
echo "=== Proposer Slots ==="
echo ""

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    if [[ "${node%_*}" != "gean" ]]; then
        echo "$node: N/A (no verified pattern)"
        continue
    fi
    slots=$(strip_ansi < "$f" | grep -F "[validator] proposed block" \
        | grep -oE "slot=[0-9]+" | cut -d= -f2 | paste -sd, || true)
    echo "$node: ${slots:-(no blocks proposed)}"
done
