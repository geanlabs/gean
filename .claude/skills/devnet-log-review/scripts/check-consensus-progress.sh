#!/bin/bash
# check-consensus-progress.sh - Show consensus progress per node
#
# Usage: check-consensus-progress.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Last slot, last justified, last finalized per node, and proposer slots

set -euo pipefail

log_dir="${1:-.}"

# Strip ANSI escape codes from input
strip_ansi() {
    sed 's/\x1b\[[0-9;]*m//g'
}

# Check if log files exist
shopt -s nullglob
log_files=("$log_dir"/*.log)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No .log files found in $log_dir" >&2
    exit 1
fi

echo "=== Consensus Progress (Last Observed) ==="
printf "%-20s %12s %12s %12s\n" "Node" "Head Slot" "Justified" "Finalized"
printf "%-20s %12s %12s %12s\n" "----" "---------" "---------" "---------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)

    # Skip combined devnet.log
    if [[ "$node" == "devnet" ]]; then
        continue
    fi

    # Extract last slot number from log (handles slot=N, slot: N, Slot N, @ N formats)
    last_slot=$(strip_ansi < "$f" | grep -oE "slot[=: ][0-9]+|Slot [0-9]+|@ [0-9]+" | grep -oE "[0-9]+" | sort -n | tail -1 || echo "0")

    if [[ -z "$last_slot" ]]; then
        last_slot="N/A"
    fi

    # Try to extract last justified / finalized
    # Patterns covered:
    #   - "justified_slot=N"      (gean, ethlambda)
    #   - "Latest Justified: Slot N"  (gean chain status box)
    #   - "Justified: slot N"     (zeam chain status / lantern variants)
    last_justified=$(strip_ansi < "$f" \
        | grep -oE "justified_slot=[0-9]+|Latest Justified:[[:space:]]+Slot[[:space:]]+[0-9]+|Justified:[[:space:]]+[Ss]lot[[:space:]:]*[0-9]+" \
        | grep -oE "[0-9]+$" | sort -n | tail -1 || echo "")
    last_finalized=$(strip_ansi < "$f" \
        | grep -oE "finalized_slot=[0-9]+|Latest Finalized:[[:space:]]+Slot[[:space:]]+[0-9]+|Finalized:[[:space:]]+[Ss]lot[[:space:]:]*[0-9]+" \
        | grep -oE "[0-9]+$" | sort -n | tail -1 || echo "")

    [[ -z "$last_justified" ]] && last_justified="N/A"
    [[ -z "$last_finalized" ]] && last_finalized="N/A"

    printf "%-20s %12s %12s %12s\n" "$node" "$last_slot" "$last_justified" "$last_finalized"
done

echo ""
echo "=== Proposer Slots ==="
echo "(Slots where each node was the proposer)"
echo ""

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    client="${node%_*}"

    # Skip combined devnet.log
    if [[ "$node" == "devnet" ]]; then
        continue
    fi

    # Extract proposed slots based on client.
    # Trailing `|| true` is required: under `set -euo pipefail` a grep with no
    # matches returns 1, which would otherwise abort the whole script and skip
    # every node alphabetically after a node with zero proposed blocks.
    case "$client" in
        gean)
            slots=$(strip_ansi < "$f" | grep "\[validator\] proposed block" | grep -oE "slot=[0-9]+" | cut -d= -f2 | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        zeam)
            slots=$(strip_ansi < "$f" | grep "produced block for slot" | grep -oE "slot=[0-9]+" | cut -d= -f2 | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        ream)
            slots=$(strip_ansi < "$f" | grep "Proposing block by Validator" | grep -oE "slot=[0-9]+" | cut -d= -f2 | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        ethlambda)
            slots=$(strip_ansi < "$f" | grep "Published block to gossipsub" | grep -oE "slot=[0-9]+" | cut -d= -f2 | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        lantern)
            slots=$(strip_ansi < "$f" | grep "[Pp]ublished block\|[Pp]roduced block" | grep -oE "slot=[0-9]+" | cut -d= -f2 | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        qlean)
            slots=$(strip_ansi < "$f" | grep "Produced block" | grep -oE "@ [0-9]+" | grep -oE "[0-9]+" | tr '\n' ',' | sed 's/,$//' || true)
            ;;
        *)
            slots=""
            ;;
    esac

    if [[ -n "$slots" ]]; then
        echo "$node: slots $slots"
    else
        echo "$node: (no blocks proposed)"
    fi
done
