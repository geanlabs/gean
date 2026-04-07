#!/bin/bash
# analyze-logs.sh - Main entry point for devnet log analysis
#
# Usage: analyze-logs.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Complete analysis summary in markdown format
# Exit codes: 0 = healthy, 1 = warnings, 2 = failed

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
log_dir="${1:-.}"

# Check if log files exist
shopt -s nullglob
log_files=("$log_dir"/*.log)
if [[ ${#log_files[@]} -eq 0 ]]; then
    echo "No .log files found in $log_dir" >&2
    exit 1
fi

# Count node log files (excluding devnet.log)
node_count=0
for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    if [[ "$node" != "devnet" ]]; then
        node_count=$((node_count + 1))
    fi
done

echo "## Devnet Log Analysis"
echo ""
echo "**Log directory:** $log_dir"
echo "**Node logs found:** $node_count"
echo ""

echo "### Errors and Warnings"
echo ""
"$SCRIPT_DIR/count-errors-warnings.sh" "$log_dir"
echo ""

echo "### Block Production"
echo ""
"$SCRIPT_DIR/count-blocks.sh" "$log_dir"
echo ""

echo "### Consensus Progress"
echo ""
"$SCRIPT_DIR/check-consensus-progress.sh" "$log_dir"
echo ""

# Calculate overall health
total_errors=0
for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)
    if [[ "$node" != "devnet" ]]; then
        errors=$(grep -i "error" "$f" 2>/dev/null | grep -cvE "manifest unknown|NoFinalizedStateFound|HandshakeTimedOut|HandshakeFailed" 2>/dev/null) || errors=0
        total_errors=$((total_errors + errors))
    fi
done

echo "---"
if [[ $total_errors -eq 0 ]]; then
    echo "**Status: HEALTHY** - No errors detected"
    exit 0
elif [[ $total_errors -lt 50 ]]; then
    echo "**Status: WARNINGS** - $total_errors total errors detected"
    exit 1
else
    echo "**Status: ISSUES** - $total_errors total errors detected (review recommended)"
    exit 2
fi
