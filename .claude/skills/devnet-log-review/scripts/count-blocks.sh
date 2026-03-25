#!/bin/bash
# count-blocks.sh - Count blocks proposed/processed per node
# Usage: ./count-blocks.sh [logs_dir]

LOGS_DIR="${1:-.}"

# ANSI color code stripping
STRIP_ANSI="sed 's/\x1b\[[0-9;]*m//g'"

LOG_FILES=$(find "$LOGS_DIR" -name "gean_*.log" -type f 2>/dev/null | sort)

if [ -z "$LOG_FILES" ]; then
    echo "No gean log files found in $LOGS_DIR"
    exit 0
fi

echo "| Node | Blocks Accepted | Blocks Synced | Sync Requests |"
echo "|------|-----------------|---------------|---------------|"

for log in $LOG_FILES; do
    NODE=$(basename "$log" .log)
    
    # Count blocks accepted via gossip
    ACCEPTED=$(eval "$STRIP_ANSI < '$log'" | grep -c "block accepted" 2>/dev/null || echo 0)
    
    # Count blocks synced
    SYNCED=$(eval "$STRIP_ANSI < '$log'" | grep -c "synced block" 2>/dev/null || echo 0)
    
    # Count BlocksByRoot requests (indicates sync activity)
    REQUESTS=$(eval "$STRIP_ANSI < '$log'" | grep -c "blocks_by_root" 2>/dev/null || echo 0)
    
    echo "| $NODE | $ACCEPTED | $SYNCED | $REQUESTS |"
done
