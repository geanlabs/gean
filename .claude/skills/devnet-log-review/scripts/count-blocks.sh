#!/bin/bash
# count-blocks.sh - Count blocks proposed and processed per node
#
# Usage: count-blocks.sh [log_dir]
#   log_dir: Directory containing *.log files (default: current directory)
#
# Output: Table with node name, blocks proposed, blocks processed
# Handles client-specific log patterns (gean, zeam, ream, lantern, ethlambda)

set -uo pipefail

log_dir="${1:-.}"

# Strip ANSI escape codes from input
strip_ansi() {
    sed 's/\x1b\[[0-9;]*m//g'
}

# Safe count function that always returns a number
count_pattern() {
    local file="$1"
    local pattern="$2"
    local result
    result=$(strip_ansi < "$file" | grep -cE "$pattern" 2>/dev/null) || result=0
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
printf "%-20s %10s %10s\n" "Node" "Proposed" "Processed"
printf "%-20s %10s %10s\n" "----" "--------" "---------"

for f in "${log_files[@]}"; do
    node=$(basename "$f" .log)

    # Skip devnet.log - it's a combined log
    if [[ "$node" == "devnet" ]]; then
        continue
    fi

    # Extract client name from node name (e.g., "gean_0" -> "gean")
    client="${node%_*}"

    proposed=0
    processed=0

    case "$client" in
        gean)
            # gean logs "[validator] proposed block" once per proposal,
            # and "[chain] block slot=N ... proc_time=Xms" for each processed block.
            proposed=$(count_pattern "$f" "\\[validator\\] proposed block")
            processed=$(count_pattern "$f" "\\[chain\\] block slot=")
            ;;
        zeam)
            proposed=$(count_pattern "$f" "produced block for slot")
            processed=$(count_pattern "$f" "processed block")
            ;;
        ream)
            # ream logs "Proposing block by Validator" when attempting
            proposed=$(count_pattern "$f" "Proposing block by Validator|Processing block built by Validator")
            processed=$(count_pattern "$f" "Processing block built")
            ;;
        lantern)
            # Lantern logs lowercase "published block" for proposals
            proposed=$(count_pattern "$f" "[Pp]roduced block|[Gg]ossiped block|[Pp]ublished block")
            processed=$(count_pattern "$f" "[Ii]mported block")
            ;;
        ethlambda)
            # ethlambda logs "Published block to gossipsub" once per block
            proposed=$(count_pattern "$f" "Published block to gossipsub")
            processed=$(count_pattern "$f" "Processed new block|Fork choice head updated")
            ;;
        qlean)
            # qlean uses "Produced block" or "Gossiped block"
            proposed=$(count_pattern "$f" "Produced block|Gossiped block")
            processed=$(count_pattern "$f" "Imported block")
            ;;
        *)
            # Unknown client - try generic patterns
            proposed=$(count_pattern "$f" "[Pp]roduced block|[Pp]ublished block|[Gg]ossiped block|proposed block")
            processed=$(count_pattern "$f" "[Pp]rocessed block|[Ii]mported block")
            ;;
    esac

    printf "%-20s %10d %10d\n" "$node" "$proposed" "$processed"
done
