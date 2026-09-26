#!/bin/bash
# count-blocks.sh - Count blocks proposed and processed per node
#
# Usage: count-blocks.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Table with node name, blocks proposed, blocks processed.
# Only gean's log patterns are verified; other clients' nodes show N/A.

set -uo pipefail

log_dir="${1:-.}"

count_pattern() {
    local result
    result=$(sed -E 's/\x1b\[[0-9;]*m//g' "$1" | grep -cF "$2") || result=0
    echo "${result:-0}"
}

shopt -s nullglob
log_files=("$log_dir"/*.log)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No .log files found in $log_dir" >&2
    exit 1
fi

printf "%-20s %10s %10s\n" "Node" "Proposed" "Processed"
printf "%-20s %10s %10s\n" "----" "--------" "---------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    [[ "$node" == "devnet" ]] && continue

    if [[ "${node%_*}" == "gean" ]]; then
        # One "[validator] proposed block" per published proposal; one
        # "[chain] block slot=" per block applied to state (own and peers').
        proposed=$(count_pattern "$f" "[validator] proposed block")
        processed=$(count_pattern "$f" "[chain] block slot=")
    else
        proposed=N/A
        processed=N/A
    fi

    printf "%-20s %10s %10s\n" "$node" "$proposed" "$processed"
done
