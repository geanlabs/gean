#!/bin/bash
# analyze-logs.sh - Main entry point for devnet log analysis
#
# Usage: analyze-logs.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Complete analysis summary in markdown format
# Exit codes: 0 = HEALTHY, 1 = UNHEALTHY, 2 = UNKNOWN (insufficient evidence)
#
# UNHEALTHY: any node log has errors (as counted by count-errors-warnings.sh).
# HEALTHY: no errors and every gean node log shows finalization advancing
#   ("[forkchoice] finalized advanced slot="). Only gean's patterns are
#   verified, so other clients' progress is not judged.
# UNKNOWN: anything else, including no node logs, no gean logs, or empty ones.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
log_dir="${1:-.}"

strip_ansi() {
    sed -E 's/\x1b\[[0-9;]*m//g'
}

# devnet.log is the combined spin-node.sh output; analyze per-node logs only.
shopt -s nullglob
node_logs=()
for f in "$log_dir"/*.log; do
    [[ "$(basename "$f")" == "devnet.log" ]] || node_logs+=("$f")
done

echo "## Devnet Log Analysis"
echo ""
echo "**Log directory:** $log_dir"
echo "**Node logs found:** ${#node_logs[@]}"
echo ""

if [[ ${#node_logs[@]} -eq 0 ]]; then
    echo "---"
    echo "**Status: UNKNOWN** - no node logs to analyze"
    exit 2
fi

echo "### Errors and Warnings"
echo ""
counts=$("$SCRIPT_DIR/count-errors-warnings.sh" "$log_dir")
echo "$counts"
echo ""

echo "### Block Production"
echo ""
"$SCRIPT_DIR/count-blocks.sh" "$log_dir"
echo ""

echo "### Consensus Progress"
echo ""
"$SCRIPT_DIR/check-consensus-progress.sh" "$log_dir"
echo ""

total_errors=$(awk 'NR > 2 { sum += $2 } END { print sum + 0 }' <<< "$counts")

gean_logs=0
no_progress=()
for f in "${node_logs[@]}"; do
    [[ "$(basename "$f")" == gean_* ]] || continue
    gean_logs=$((gean_logs + 1))
    strip_ansi < "$f" | grep -F "[forkchoice] finalized advanced slot=" > /dev/null \
        || no_progress+=("$(basename "$f" .log)")
done

echo "---"
if [[ $total_errors -gt 0 ]]; then
    echo "**Status: UNHEALTHY** - $total_errors errors detected"
    exit 1
elif [[ $gean_logs -eq 0 ]]; then
    echo "**Status: UNKNOWN** - no gean node logs; other clients' progress is not verified"
    exit 2
elif [[ ${#no_progress[@]} -gt 0 ]]; then
    echo "**Status: UNKNOWN** - no finalization progress evidence in: ${no_progress[*]}"
    exit 2
fi
echo "**Status: HEALTHY** - no errors; finalization advanced on every gean node"
