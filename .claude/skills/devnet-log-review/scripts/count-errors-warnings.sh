#!/bin/bash
# count-errors-warnings.sh - Count errors and warnings per node log file
#
# Usage: count-errors-warnings.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Table with node name, error count, warning count
# Excludes benign patterns like "manifest unknown", "NoFinalizedStateFound", "TODO"

set -uo pipefail

log_dir="${1:-.}"

# Benign patterns to exclude from counts
BENIGN_ERRORS="manifest unknown|NoFinalizedStateFound|HandshakeTimedOut|HandshakeFailed|connection refused"
BENIGN_WARNINGS="TODO|deprecated"

# Safe count function
count_filtered() {
    local file="$1"
    local pattern="$2"
    local exclude="$3"
    local result
    result=$(grep -i "$pattern" "$file" 2>/dev/null | grep -cvE "$exclude" 2>/dev/null) || result=0
    echo "${result:-0}"
}

# Check if log files exist
shopt -s nullglob
log_files=("$log_dir"/*.log)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No .log files found in $log_dir" >&2
    exit 1
fi

# Print header
printf "%-20s %8s %8s\n" "Node" "Errors" "Warnings"
printf "%-20s %8s %8s\n" "----" "------" "--------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)

    # Count errors excluding benign patterns
    errors=$(count_filtered "$f" "error" "$BENIGN_ERRORS")

    # Count warnings excluding benign patterns
    warnings=$(count_filtered "$f" "warn" "$BENIGN_WARNINGS")

    printf "%-20s %8d %8d\n" "$node" "$errors" "$warnings"
done
