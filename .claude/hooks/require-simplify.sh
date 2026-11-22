#!/bin/sh
# PreToolUse hook: blocks a `git commit` while gean has changes the simplify skill hasn't
# reviewed. The skill records the reviewed change in .git/simplify-reviewed.
input=$(cat)
case $input in *commit*) ;; *) exit 0 ;; esac
printf '%s' "$input" | jq -r '.tool_input.command' |
	grep -Eq '(^|[;&|(]|[[:space:]])git([[:space:]]+-[cC][[:space:]]+[^[:space:]]+)*[[:space:]]+commit([[:space:]]|$)' || exit 0

cd "$(dirname "$0")/../.." || exit 0
id=$(.agents/skills/simplify/change-id.sh) || exit 0
[ "$id" = "$(git rev-parse 'HEAD^{tree}')" ] && exit 0
[ "$(cat "$(git rev-parse --git-dir)/simplify-reviewed" 2>/dev/null)" = "$id" ] && exit 0

echo "The simplify skill (.agents/skills/simplify/SKILL.md) has not reviewed the current change; any edit after its last run needs a new review. Run it, then commit." >&2
exit 2
