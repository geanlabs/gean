#!/bin/bash
# run.sh - Regression checks for the devnet-log-review scripts.
#
# Builds tiny synthetic logs in gean's colored format (internal/logger) and
# asserts verdicts, exit codes, and outputs. Usage: scripts/test/run.sh

set -uo pipefail

SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failures=0

# gean <LEVEL> <component> <message>: one log line as internal/logger emits it.
gean() {
    local color=32
    [[ $1 == WARN ]] && color=33
    [[ $1 == ERROR ]] && color=31
    printf '\033[2m2026-01-01T00:00:00.000Z\033[0m \033[1m\033[%sm%s\033[0m \033[36m[%s]\033[0m %s\n' \
        "$color" "$1" "$2" "$3"
}

check() {
    local name=$1 want_code=$2 want_text=$3 dir=$4 script=$5
    local out code
    out=$("$SCRIPTS/$script" "$dir" 2>&1)
    code=$?
    if [[ $code -ne $want_code ]] || ! grep -qF -- "$want_text" <<< "$out"; then
        echo "FAIL $name: exit $code (want $want_code), want output containing '$want_text'"
        echo "$out" | sed 's/^/    /'
        failures=$((failures + 1))
    else
        echo "ok   $name"
    fi
}

head_event() { # slot justified finalized
    gean INFO forkchoice "head slot=$1 head_root=0xaa parent_root=0xbb justified_slot=$2 justified_root=0xcc finalized_slot=$3 finalized_root=0xdd"
}

mkdir "$tmp/none" "$tmp/empty" "$tmp/errors" "$tmp/healthy"

# Combined devnet.log errors must not count; an empty node log is no evidence.
gean ERROR node "fatal: from combined log" > "$tmp/empty/devnet.log"
: > "$tmp/empty/gean_0.log"

# gean_0 has no errors, so show-errors must continue to gean_1.
{ head_event 4 2 0; gean INFO forkchoice "finalized advanced slot=2 root=0xdd"; } > "$tmp/errors/gean_0.log"
{ head_event 4 2 0; gean ERROR chain "block processing failed slot=4 block_root=0xee: bad"; } > "$tmp/errors/gean_1.log"

# The last head event (a reorg to a lower slot) must win over the max slot.
{
    head_event 10 8 6
    gean INFO forkchoice "finalized advanced slot=6 root=0xdd"
    gean INFO validator "proposed block slot=10 block_root=0xaa attestations=3"
    gean WARN forkchoice "REORG depth=1 slot=9 head_root=0xaa parent_root=0xbb (was 0xcc) justified_slot=7 justified_root=0xcc finalized_slot=5 finalized_root=0xdd"
} > "$tmp/healthy/gean_0.log"
# Another client in the same run: its progress is not judged, so HEALTHY still holds.
echo "zeam node started slot=10" > "$tmp/healthy/zeam_0.log"

check "no logs -> UNKNOWN"            2 "Status: UNKNOWN"   "$tmp/none"    analyze-logs.sh
check "devnet.log + empty -> UNKNOWN" 2 "Status: UNKNOWN"   "$tmp/empty"   analyze-logs.sh
check "errors -> UNHEALTHY"           1 "Status: UNHEALTHY" "$tmp/errors"  analyze-logs.sh
check "finalizing -> HEALTHY"         0 "Status: HEALTHY"   "$tmp/healthy" analyze-logs.sh
check "progress is last observation"  0 "$(printf '%-20s %12s %12s %12s' gean_0 9 7 5)" \
    "$tmp/healthy" check-consensus-progress.sh
check "show-errors past clean node"   0 "block processing failed" "$tmp/errors" show-errors.sh

[[ $failures -eq 0 ]] && echo "all passed" || { echo "$failures failed"; exit 1; }
