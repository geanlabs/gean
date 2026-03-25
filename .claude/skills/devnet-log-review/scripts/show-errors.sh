#!/bin/bash
# show-errors.sh - Display error details
# Usage: ./show-errors.sh [-n node] [-l limit] [-w] [logs_dir]
#   -n: Filter by node name (e.g., gean_0)
#   -l: Limit output lines (default: 20)
#   -w: Include warnings (default: errors only)

set -e

NODE_FILTER=""
LIMIT=20
SHOW_WARNINGS=false
LOGS_DIR="."

while getopts "n:l:wh" opt; do
    case $opt in
        n) NODE_FILTER="$OPTARG" ;;
        l) LIMIT="$OPTARG" ;;
        w) SHOW_WARNINGS=true ;;
        h) echo "Usage: $0 [-n node] [-l limit] [-w] [logs_dir]"
           echo "  -n: Filter by node name"
           echo "  -l: Limit output lines (default: 20)"
           echo "  -w: Include warnings"
           exit 0 ;;
    esac
done
shift $((OPTIND-1))
[ -n "$1" ] && LOGS_DIR="$1"

STRIP_ANSI="sed 's/\x1b\[[0-9;]*m//g'"

if [ -n "$NODE_FILTER" ]; then
    LOG_FILES=$(find "$LOGS_DIR" -name "${NODE_FILTER}.log" -type f 2>/dev/null)
else
    LOG_FILES=$(find "$LOGS_DIR" -name "gean_*.log" -type f 2>/dev/null | sort)
fi

if [ -z "$LOG_FILES" ]; then
    echo "No log files found"
    exit 0
fi

echo "=== Errors (limit: $LIMIT) ==="

for log in $LOG_FILES; do
    NODE=$(basename "$log" .log)
    echo ""
    echo "--- $NODE ---"
    eval "$STRIP_ANSI < '$log'" | grep -E "ERR|panic|fatal" | tail -"$LIMIT"
done

if [ "$SHOW_WARNINGS" = true ]; then
    echo ""
    echo "=== Warnings (limit: $LIMIT) ==="
    
    for log in $LOG_FILES; do
        NODE=$(basename "$log" .log)
        echo ""
        echo "--- $NODE ---"
        eval "$STRIP_ANSI < '$log'" | grep "WRN" | tail -"$LIMIT"
    done
fi
