#!/bin/bash
# show-errors.sh - Display error details for investigation
#
# Usage: show-errors.sh [options] [log_dir]
#   -n NODE   Filter to specific node (e.g., "gean_0")
#   -l LIMIT  Limit number of errors shown per file (default: 20)
#   -w        Also show warnings
#   log_dir   Directory containing *.log files (default: current directory)
#
# Output: Error messages from log files, stripped of ANSI codes

set -euo pipefail

# Defaults
node_filter=""
limit=20
show_warnings=false
log_dir="."

# Parse options
while getopts "n:l:w" opt; do
    case $opt in
        n) node_filter="$OPTARG" ;;
        l) limit="$OPTARG" ;;
        w) show_warnings=true ;;
        *) echo "Usage: $0 [-n node] [-l limit] [-w] [log_dir]" >&2; exit 1 ;;
    esac
done
shift $((OPTIND-1))

# Remaining argument is log_dir
if [[ $# -gt 0 ]]; then
    log_dir="$1"
fi

# Strip ANSI escape codes from input
strip_ansi() {
    sed 's/\x1b\[[0-9;]*m//g'
}

# Build file pattern
if [[ -n "$node_filter" ]]; then
    pattern="$log_dir/${node_filter}.log"
else
    pattern="$log_dir/*.log"
fi

# Check if log files exist
shopt -s nullglob
log_files=($pattern)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No matching .log files found" >&2
    exit 1
fi

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)

    # Skip combined devnet.log unless specifically requested
    if [[ "$node" == "devnet" && -z "$node_filter" ]]; then
        continue
    fi

    echo "=== $node ==="

    # Show errors
    error_count=$(strip_ansi < "$f" | grep -ci "error" || echo 0)
    echo "Errors ($error_count total, showing first $limit):"
    strip_ansi < "$f" | grep -i "error" | head -"$limit"

    # Optionally show warnings
    if $show_warnings; then
        echo ""
        warning_count=$(strip_ansi < "$f" | grep -ci "warn" || echo 0)
        echo "Warnings ($warning_count total, showing first $limit):"
        strip_ansi < "$f" | grep -i "warn" | head -"$limit"
    fi

    echo ""
done
