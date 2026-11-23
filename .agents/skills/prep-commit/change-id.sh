#!/bin/sh
# Prints the tree the working copy would commit, leaving out Markdown files, which need no
# review. Staging does not change it, and it is HEAD's tree when nothing else changed.
cd "$(git rev-parse --show-toplevel)" || exit 1
GIT_INDEX_FILE="$(git rev-parse --absolute-git-dir)/prep-commit-index"
export GIT_INDEX_FILE
git read-tree HEAD && git add -A -- . ':!*.md' && git write-tree
