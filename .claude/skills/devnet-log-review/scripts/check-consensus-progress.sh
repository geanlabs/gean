#!/bin/bash
# check-consensus-progress.sh - Show consensus state per node
# Usage: ./check-consensus-progress.sh [logs_dir]

LOGS_DIR="${1:-.}"

STRIP_ANSI="sed 's/\x1b\[[0-9;]*m//g'"
LOG_FILES=$(find "$LOGS_DIR" -name "gean_*.log" -type f 2>/dev/null | sort)

if [ -z "$LOG_FILES" ]; then
    echo "No gean log files found in $LOGS_DIR"
    exit 0
fi

echo "| Node | Last Slot | Head Slot | Finalized | Justified | Peers |"
echo "|------|-----------|-----------|-----------|-----------|-------|"

for log in $LOG_FILES; do
    NODE=$(basename "$log" .log)
    
    # Get latest slot log line
    LAST=$(eval "$STRIP_ANSI < '$log'" | grep " slot=" | tail -1 2>/dev/null || echo "")
    
    # Extract values
    SLOT=$(echo "$LAST" | grep -oE "slot=[0-9]+" | tail -1 | cut -d= -f2 || echo "-")
    HEAD=$(echo "$LAST" | grep -oE "head_slot=[0-9]+" | tail -1 | cut -d= -f2 || echo "-")
    FINALIZED=$(echo "$LAST" | grep -oE "finalized_slot=[0-9]+" | tail -1 | cut -d= -f2 || echo "-")
    JUSTIFIED=$(echo "$LAST" | grep -oE "justified_slot=[0-9]+" | tail -1 | cut -d= -f2 || echo "-")
    PEERS=$(echo "$LAST" | grep -oE "peers=[0-9]+" | tail -1 | cut -d= -f2 || echo "-")
    
    echo "| $NODE | $SLOT | $HEAD | $FINALIZED | $JUSTIFIED | $PEERS |"
done
