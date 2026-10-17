#!/usr/bin/env bash
# Lists the lean-review scope as markdown: committed (<base>...HEAD), staged,
# unstaged, and untracked files, each labeled with its source.
# Usage: collect-diff.sh [base]   (default: main)
set -euo pipefail

BASE="${1:-main}"
cd "$(git rev-parse --show-toplevel)"
if ! git rev-parse --verify --quiet "$BASE^{commit}" >/dev/null; then
    echo "error: base '$BASE' does not exist" >&2
    exit 1
fi

# Hard-skip patterns from references/dont-touch.md. Read via ENVIRON so awk
# does not process the backslashes.
export SKIP='(_encoding\.go$|^xmss/rust/|\.py$|^internal/specfixtures/|^internal/spectests/fixture\.go$|^vendor/|^third_party/|^external/)'

# Rows: source<TAB>added<TAB>removed<TAB>path. --no-renames lists a rename as
# delete + add, so the path is always the fourth tab-separated field.
label() { awk -F'\t' -v OFS='\t' -v s="$1" '{ print s, $1, $2, $3 }'; }
g() { git -c core.quotePath=false "$@"; }
ROWS="$(
    g diff --numstat --no-renames "$BASE...HEAD" | label committed
    g diff --numstat --no-renames --cached | label staged
    g diff --numstat --no-renames | label unstaged
    g ls-files --others --exclude-standard | while IFS= read -r f; do
        if [ -f "$f" ] && [ ! -L "$f" ]; then n="$(wc -l <"$f" | tr -d ' ')"; else n=-; fi
        printf 'untracked\t%s\t0\t%s\n' "$n" "$f"
    done
)"

COMMITS="$(git rev-list --count "$BASE..HEAD")"
echo "## /lean-review scope: \`$(git rev-parse --abbrev-ref HEAD)\` vs \`$BASE\`"
echo
echo "- Merge base \`$(git merge-base "$BASE" HEAD)\`, $COMMITS commit(s) ahead"
echo
if [ -z "$ROWS" ]; then
    echo "**Nothing to review.** No committed, staged, unstaged, or untracked changes."
    exit 0
fi

echo "### Files (most lines added first)"
echo
printf '%s\n' "$ROWS" | sort -t$'\t' -k2,2nr | awk -F'\t' '{
    printf "- [%s] `%s` (+%s / -%s)%s\n", $1, $4, $2, $3, ($4 ~ ENVIRON["SKIP"] ? " (hard-skip)" : "")
}'
REVIEWABLE="$(printf '%s\n' "$ROWS" | awk -F'\t' '$4 !~ ENVIRON["SKIP"]' | wc -l | tr -d ' ')"
echo
echo "- $REVIEWABLE entries remain after the don't-touch filter"
if [ "$REVIEWABLE" -eq 0 ]; then
    echo
    echo "**Nothing to review.** Every change is in don't-touch paths."
    exit 0
fi

if [ "$COMMITS" -gt 0 ]; then
    echo
    echo "### Commits"
    echo
    echo '```'
    git log --oneline "$BASE..HEAD"
    echo '```'
fi
