#!/usr/bin/env bash
# Regression tests for collect-diff.sh in throwaway git repos.
# Usage: test-collect-diff.sh [path/to/collect-diff.sh]
set -euo pipefail

SCRIPT="$(realpath "${1:-$(dirname "$0")/collect-diff.sh}")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
FAILED=0

# new_repo <name>: repo on main with one commit containing modified.go.
new_repo() {
    git init -q -b main "$TMP/$1"
    echo a >"$TMP/$1/modified.go"
    git -C "$TMP/$1" add . && git -C "$TMP/$1" commit -q -m init
}

# expect <name> <dir> <pattern>...: script exits 0 and prints every pattern.
expect() {
    local name="$1" dir="$2" out rc=0 p
    shift 2
    out="$(cd "$dir" && "$SCRIPT" 2>&1)" || rc=$?
    [ "$rc" -eq 0 ] || { echo "FAIL $name: exit $rc"; FAILED=1; return; }
    for p in "$@"; do
        grep -qF -- "$p" <<<"$out" || { echo "FAIL $name: missing '$p'"; FAILED=1; return; }
    done
    echo "ok   $name"
}

new_repo excluded
git -C "$TMP/excluded" checkout -q -b feat
mkdir -p "$TMP/excluded/internal/types"
echo x >"$TMP/excluded/internal/types/foo_encoding.go"
git -C "$TMP/excluded" add . && git -C "$TMP/excluded" commit -q -m gen
expect "all-excluded diff" "$TMP/excluded" "Nothing to review"

new_repo worktree
echo b >>"$TMP/worktree/modified.go"
echo n >"$TMP/worktree/added.go"
expect "uncommitted with 0 commits ahead" "$TMP/worktree" '[unstaged] `modified.go`' '[untracked] `added.go`'

exit "$FAILED"
