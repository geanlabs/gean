#!/usr/bin/env bash
# Prints the start, the error context and the end of each gean container log
# from a hive run, so a failed run can be read in the job log.
set -euo pipefail
shopt -s nullglob

clip_width() {
    cut -c 1-2000
}

for log in "$1"/gean*/client-*.log; do
    echo "::group::$log"
    echo "lines=$(wc -l < "$log") bytes=$(wc -c < "$log")"

    echo "first 200 lines:"
    sed -n '1,200p' "$log" | clip_width

    echo "error context:"
    grep -n -i -E -C 20 \
        'panic|fatal|error|warn|oom|out of memory|killed|no route to host|connection refused|cannot allocate' \
        "$log" | head -n 2000 | clip_width || true

    echo "last 300 lines:"
    tail -n 300 "$log" | clip_width

    echo "::endgroup::"
done
