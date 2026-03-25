#!/bin/bash
# analyze-logs.sh - Main entry point for devnet log analysis
# Usage: ./analyze-logs.sh [logs_dir]
# If no directory provided, uses current directory

set -e

LOGS_DIR="${1:-.}"

echo "## Devnet Log Analysis"
echo ""
echo "**Directory:** $LOGS_DIR"
echo ""

# Count errors and warnings
echo "### Errors & Warnings"
./.claude/skills/devnet-log-review/scripts/count-errors-warnings.sh "$LOGS_DIR"
echo ""

# Count blocks
echo "### Block Production"
./.claude/skills/devnet-log-review/scripts/count-blocks.sh "$LOGS_DIR"
echo ""

# Consensus progress
echo "### Consensus Progress"
./.claude/skills/devnet-log-review/scripts/check-consensus-progress.sh "$LOGS_DIR"
echo ""

# Generate summary
echo "### Summary"

# Get total errors and warnings
TOTAL_ERRORS=$(grep -r "ERR\|panic\|fatal" "$LOGS_DIR" --include="*.log" 2>/dev/null | grep -v "peer disconnected" | wc -l)
TOTAL_WARNINGS=$(grep -r "WRN\|warning\|skipping validator duties" "$LOGS_DIR" --include="*.log" 2>/dev/null | wc -l)
TOTAL_BLOCKS=$(grep -r "block accepted\|synced block" "$LOGS_DIR" --include="gean_*.log" 2>/dev/null | wc -l)

echo "| Metric | Count |"
echo "|--------|-------|"
echo "| Errors | $TOTAL_ERRORS |"
echo "| Warnings | $TOTAL_WARNINGS |"
echo "| Blocks Processed | $TOTAL_BLOCKS |"
echo ""

# Determine status
if [ "$TOTAL_ERRORS" -eq 0 ] && [ "$TOTAL_BLOCKS" -gt 0 ]; then
    echo "🟢 PASSED - All nodes healthy, consensus achieved"
elif [ "$TOTAL_BLOCKS" -gt 0 ]; then
    echo "🟡 PASSED WITH WARNINGS - Consensus working but minor issues detected"
else
    echo "🔴 FAILED - Consensus broken or no blocks processed"
fi
