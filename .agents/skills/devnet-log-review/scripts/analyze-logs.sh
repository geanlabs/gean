#!/bin/bash
# Summarize a devnet run from gean's node logs: per node, blocks proposed and imported, the head,
# justified and finalized slots from the last CHAIN STATUS block, and error and warning counts.
#
# Usage: analyze-logs.sh [-e N] [log_dir]
#   -e N     also print each node's first N error and warning lines
#   log_dir  directory holding gean_<n>.log files (default: current directory)

set -uo pipefail

show=0
if [[ "${1:-}" == "-e" ]]; then
    show=${2:?-e needs a line count}
    shift 2
fi
dir=${1:-.}

# Normal when occasional: a block for the slot arrived before this node's own proposal.
benign='discarding proposal slot=[0-9]+: head moved'

shopt -s nullglob
logs=("$dir"/gean_*.log)
if [[ ${#logs[@]} -eq 0 ]]; then
    echo "no gean_*.log files in $dir" >&2
    exit 1
fi

printf '%-10s %8s %8s %6s %9s %9s %6s %8s\n' \
    Node Proposed Imported Head Justified Finalized Errors Warnings
total=0
for f in "${logs[@]}"; do
    read -r proposed imported head justified finalized errors warnings < <(
        awk -v benign="$benign" '
            / \[validator\] proposed block / { p++ }
            / \[chain\] block slot=/ { i++ }
            /CHAIN STATUS:/ { match($0, /Head Slot: [0-9]+/); h = substr($0, RSTART + 11, RLENGTH - 11) }
            /Latest Justified:/ { j = $4 }
            /Latest Finalized:/ { fin = $4 }
            / ERROR \[/ && $0 !~ benign { e++ }
            / WARN \[/ && $0 !~ benign { w++ }
            END {
                printf "%d %d %s %s %s %d %d\n", p, i, (h == "" ? "-" : h),
                    (j == "" ? "-" : j), (fin == "" ? "-" : fin), e, w
            }' "$f")
    printf '%-10s %8d %8d %6s %9s %9s %6d %8d\n' "$(basename "$f" .log)" \
        "$proposed" "$imported" "$head" "$justified" "$finalized" "$errors" "$warnings"
    total=$((total + errors))
done

if ((show > 0)); then
    for f in "${logs[@]}"; do
        echo
        echo "=== $(basename "$f" .log)"
        awk -v benign="$benign" -v n="$show" '/ (ERROR|WARN) \[/ && $0 !~ benign && c++ < n' "$f"
    done
fi

echo
echo "Total errors: $total"
