#!/bin/bash
# count-errors-warnings.sh - Count errors and warnings per node log file
#
# Usage: count-errors-warnings.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Table with node name, error count, warning count
# Skips the combined devnet.log. Benign patterns excluded from the counts are
# listed in references/ERROR_CLASSIFICATION.md ("Expected/Benign Messages").

set -uo pipefail

log_dir="${1:-.}"

BENIGN_ERRORS="NoFinalizedStateFound|HandshakeTimedOut"
BENIGN_WARNINGS="TODO"

count_filtered() {
    local file="$1"
    local pattern="$2"
    local exclude="$3"
    local result
    result=$(sed -E 's/\x1b\[[0-9;]*m//g' "$file" | grep -i "$pattern" | grep -cvE "$exclude") || result=0
    echo "${result:-0}"
}

shopt -s nullglob
log_files=("$log_dir"/*.log)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No .log files found in $log_dir" >&2
    exit 1
fi

printf "%-20s %8s %8s\n" "Node" "Errors" "Warnings"
printf "%-20s %8s %8s\n" "----" "------" "--------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    [[ "$node" == "devnet" ]] && continue

    errors=$(count_filtered "$f" "error" "$BENIGN_ERRORS")
    warnings=$(count_filtered "$f" "warn" "$BENIGN_WARNINGS")

    printf "%-20s %8d %8d\n" "$node" "$errors" "$warnings"
done
