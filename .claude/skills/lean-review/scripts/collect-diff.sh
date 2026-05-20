#!/usr/bin/env bash
# Collect diff vs a base branch as structured input for the lean-review skill.
# Usage: collect-diff.sh [base-branch]  (default: main)
#
# Output is markdown; the skill reads it to bound the review scope.
set -euo pipefail

BASE="${1:-main}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! git rev-parse --verify --quiet "$BASE" >/dev/null; then
    echo "error: base branch '$BASE' does not exist" >&2
    exit 1
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
MERGE_BASE="$(git merge-base "$BASE" HEAD)"
COMMITS_AHEAD="$(git rev-list --count "$BASE..HEAD")"

if [ "$COMMITS_AHEAD" -eq 0 ]; then
    echo "## /lean-review — nothing to review"
    echo ""
    echo "Current branch \`$CURRENT_BRANCH\` has no commits ahead of \`$BASE\`."
    exit 0
fi

echo "## /lean-review — diff scope"
echo ""
echo "- Base branch: \`$BASE\`"
echo "- Current branch: \`$CURRENT_BRANCH\`"
echo "- Merge base: \`$MERGE_BASE\`"
echo "- Commits ahead: $COMMITS_AHEAD"
echo ""

echo "### Totals"
echo ""
echo '```'
git diff --shortstat "$BASE...HEAD"
echo '```'
echo ""

echo "### Changed files (+added / -removed)"
echo ""
git diff --numstat "$BASE...HEAD" \
    | awk '{ printf "- `%s` (+%s / -%s)\n", $3, $1, $2 }'
echo ""

NEW_FILES="$(git diff --diff-filter=A --name-only "$BASE...HEAD" || true)"
if [ -n "$NEW_FILES" ]; then
    echo "### New files"
    echo ""
    while IFS= read -r f; do
        echo "- \`$f\`"
    done <<< "$NEW_FILES"
    echo ""
fi

DELETED_FILES="$(git diff --diff-filter=D --name-only "$BASE...HEAD" || true)"
if [ -n "$DELETED_FILES" ]; then
    echo "### Deleted files"
    echo ""
    while IFS= read -r f; do
        echo "- \`$f\`"
    done <<< "$DELETED_FILES"
    echo ""
fi

echo "### Biggest additions (top 10 by lines added)"
echo ""
echo '```'
git diff --numstat "$BASE...HEAD" \
    | awk '$1 ~ /^[0-9]+$/ { printf "%d\t+%s / -%s\t%s\n", $1, $1, $2, $3 }' \
    | sort -rn \
    | head -10 \
    | cut -f2-
echo '```'
echo ""

echo "### Don't-touch matches (excluded from review)"
echo ""
# Mirror references/dont-touch.md hard-skip patterns.
DONT_TOUCH_PATTERNS='(_encoding\.go$|^xmss/rust/|\.py$|^specfixtures/|^spectests/fixture\.go$|^vendor/|^third_party/|^external/)'
SKIPPED="$(git diff --name-only "$BASE...HEAD" | grep -E "$DONT_TOUCH_PATTERNS" || true)"
if [ -n "$SKIPPED" ]; then
    while IFS= read -r f; do
        echo "- \`$f\` (hard-skip)"
    done <<< "$SKIPPED"
else
    echo "- (none)"
fi
echo ""

REVIEWABLE_COUNT="$(git diff --name-only "$BASE...HEAD" | grep -Ev "$DONT_TOUCH_PATTERNS" | wc -l | tr -d ' ')"
echo "### Reviewable file count"
echo ""
echo "- $REVIEWABLE_COUNT file(s) remain after don't-touch filter"
echo ""

if [ "$REVIEWABLE_COUNT" -eq 0 ]; then
    echo "**Nothing to review.** Entire diff is in don't-touch paths."
    exit 0
fi

echo "### Commit log"
echo ""
echo '```'
git log --oneline "$BASE..HEAD"
echo '```'
