#!/bin/sh
# PreToolUse hook: blocks a `git commit` while gean has changes that haven't been through the
# prep-commit skill. The skill records the change it prepared in .git/prep-commit-done.
input=$(cat)
case $input in *commit*) ;; *) exit 0 ;; esac
printf '%s' "$input" | jq -r '.tool_input.command' |
	grep -Eq '(^|[;&|(]|[[:space:]])git([[:space:]]+-[cC][[:space:]]+[^[:space:]]+)*[[:space:]]+commit([[:space:]]|$)' || exit 0

cd "$(dirname "$0")/../.." || exit 0
id=$(.agents/skills/prep-commit/change-id.sh) || exit 0
[ "$id" = "$(git rev-parse 'HEAD^{tree}')" ] && exit 0
[ "$(cat "$(git rev-parse --git-dir)/prep-commit-done" 2>/dev/null)" = "$id" ] && exit 0

echo "The current change hasn't been through the prep-commit skill (.agents/skills/prep-commit/SKILL.md) since its last edit. Run it, then commit." >&2
exit 2
