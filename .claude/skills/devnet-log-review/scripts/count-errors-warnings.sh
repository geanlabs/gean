#!/bin/bash
# count-errors-warnings.sh - Count errors and warnings per node
# Usage: ./count-errors-warnings.sh [logs_dir]

LOGS_DIR="${1:-.}"

# ANSI color code stripping
STRIP_ANSI="sed 's/\x1b\[[0-9;]*m//g'"

# Find all gean log files
LOG_FILES=$(find "$LOGS_DIR" -name "gean_*.log" -type f 2>/dev/null | sort)

if [ -z "$LOG_FILES" ]; then
    echo "No gean log files found in $LOGS_DIR"
    exit 0
fi

echo "| Node | Errors | Warnings | Behind Peers |"
echo "|------|--------|----------|--------------|"

for log in $LOG_FILES; do
    NODE=$(basename "$log" .log)
    
    # Count errors (exclude benign disconnection messages)
    ERRORS=$(eval "$STRIP_ANSI < '$log'" | grep -c "ERR\|panic\|fatal" 2>/dev/null || echo 0)
    
    # Count warnings
    WARNINGS=$(eval "$STRIP_ANSI < '$log'" | grep -c "WRN\|warning" 2>/dev/null || echo 0)
    
    # Count "behind peers" occurrences
    BEHIND=$(eval "$STRIP_ANSI < '$log'" | grep -c "skipping validator duties while behind peers" 2>/dev/null || echo 0)
    
    echo "| $NODE | $ERRORS | $WARNINGS | $BEHIND |"
done
