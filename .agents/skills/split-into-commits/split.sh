#!/usr/bin/env bash
# Git plumbing for the split-into-commits skill. It writes to the index and to its own state
# directory, never to a tracked file in the user's working tree.
set -euo pipefail

top=$(git rev-parse --show-toplevel)
cd "$top"
state=${TMPDIR:-/tmp}/split-into-commits/$(basename "$top")
snap=$state/snapshot
tree_dir=$state/tree
mkdir -p "$state"

fail() { echo "split.sh: $*" >&2; exit 1; }

# Every path the change touches, minus SPLIT_EXCLUDE (an extended regex for unrelated paths such
# as a scratch directory). grep exits 1 when it filters out everything, which is not an error.
change() {
    { git diff --name-only HEAD; git ls-files --others --exclude-standard; } | sort -u |
        { grep -Ev "${SPLIT_EXCLUDE:-^$}" || (($? == 1)); }
}

need_snapshot() { [[ -f $state/files ]] || fail "no snapshot; run 'split.sh snapshot' first"; }

stage_file() {
    local mode=100644
    [[ -x $2 ]] && mode=100755
    git update-index --add --cacheinfo "$mode,$(git hash-object -w "$2"),$1"
}

# Checks out a tree in a detached worktree that persists between runs, so the XMSS library's
# cargo output inside it survives and make rebuilds only what changed. A worktree checks out
# commits, not trees, so the tree gets a throwaway commit.
checkout_tree() {
    local commit
    commit=$(git -c user.name=split -c user.email=split@localhost commit-tree "$1" -m split)
    if [[ ! -f $tree_dir/.git ]]; then
        rm -rf "$tree_dir"
        git worktree prune
        git worktree add -q --detach "$tree_dir" "$commit"
    fi
    git -C "$tree_dir" checkout -q --detach -f "$commit"
    git -C "$tree_dir" clean -fdq
}

# The whole change as a tree, built in a scratch index so the real one is left alone.
change_tree() {
    local index=$state/change.index
    rm -f "$index"
    GIT_INDEX_FILE=$index git read-tree HEAD
    change | tr '\n' '\0' | GIT_INDEX_FILE=$index git update-index --add --remove -z --stdin
    GIT_INDEX_FILE=$index git write-tree
}

passed() { grep -c '^--- PASS' "$state/test.log" || true; }

cmd_snapshot() {
    if [[ -d $snap ]] && ! git diff --cached --quiet; then
        fail "a unit is staged; land or unstage it before taking a new snapshot"
    fi
    local files f count=0
    files=$(change)
    rm -rf "$snap"
    echo "$files" | sed "/^$/d" > "$state/files"
    while read -r f; do
        [[ -f $f ]] || continue
        mkdir -p "$snap/$(dirname "$f")"
        cp -p "$f" "$snap/$f"
        count=$((count + 1))
    done < "$state/files"
    git rev-parse HEAD > "$state/base"
    git reset -q
    echo "snapshot: $count files in $snap"
    echo "base:     $(cat "$state/base") on $(git rev-parse --abbrev-ref HEAD)"
    echo "index:    empty"
}

cmd_drift() {
    need_snapshot
    local base now bad=0 f
    base=$(cat "$state/base")
    now=$(git rev-parse HEAD)
    if [[ $base == "$now" ]]; then
        echo "base: unchanged at $now"
    elif git merge-base --is-ancestor "$base" "$now"; then
        echo "base: $(git rev-list --count "$base..$now") landed commit(s) on top, now $now"
    else
        echo "base: MOVED from $base to $now, which does not descend from it"
        bad=1
    fi
    while read -r f; do
        [[ -f $snap/$f ]] || continue
        if [[ ! -f $f ]]; then
            echo "  gone from the worktree: $f"
            bad=1
        elif ! cmp -s "$snap/$f" "$f"; then
            echo "  differs from the snapshot: $f"
            bad=1
        fi
    done < "$state/files"
    if ((bad == 0)); then
        echo "worktree: matches the snapshot"
    else
        echo "worktree or base moved: re-snapshot before going on"
    fi
    return "$bad"
}

cmd_stale() {
    # Doesn't fetch: without a fresh fetch, origin's branch and the PR state are old and the
    # verdict can be wrong.
    local main branch pr="" behind overlap prefix bad=0
    main=$(git symbolic-ref --short refs/remotes/origin/HEAD 2> /dev/null || echo origin/main)
    branch=$(git rev-parse --abbrev-ref HEAD)
    echo "branch:   $branch at $(git rev-parse --short HEAD)"
    if command -v gh > /dev/null; then
        pr=$(gh pr list --head "$branch" --state all --json number,state \
            --jq 'first | select(.) | "\(.number) \(.state)"' 2> /dev/null || true)
        echo "pr:       ${pr:-none}"
        [[ $pr == *OPEN* || -z $pr ]] || bad=1
    fi
    behind=$(git rev-list --count "HEAD..$main")
    echo "default:  $main is $behind commit(s) ahead of HEAD"
    ((behind == 0)) || bad=1
    overlap=$(comm -12 <(change) <(git diff --name-only "$(git merge-base HEAD "$main")" "$main" | sort))
    echo "overlap:  ${overlap:-none}" | sed '2,$s/^/          /'
    prefix=$(git branch --sort=-committerdate --format='%(refname:short)' | grep -om1 '^[^/]*/' || true)
    echo "prefix:   ${prefix:-none}"
    if ((bad == 0)); then
        echo "verdict:  current"
    else
        echo "verdict:  STALE; propose git checkout --no-track -b ${prefix}<name> $main"
    fi
    return "$bad"
}

cmd_stage() {
    local path=${1:?usage: split.sh stage <path> [<from>]}
    local from=${2:-$snap/$path}
    if [[ -f $state/files ]] && ! grep -qxF "$path" "$state/files"; then
        fail "$path is not part of the change"
    fi
    [[ -f $from ]] || fail "nothing to stage for $path at $from"
    stage_file "$path" "$from"
    echo "staged $path from ${2:-the snapshot}"
}

cmd_lock() {
    need_snapshot
    [[ -f $snap/go.mod || -f $snap/go.sum ]] || fail "the change touches neither go.mod nor go.sum"
    checkout_tree "$(git write-tree)"
    (cd "$tree_dir" && go mod tidy) || fail "go mod tidy failed on the staged tree"
    local f
    for f in go.mod go.sum; do
        stage_file "$f" "$tree_dir/$f"
        echo "staged $f as resolved for the staged tree; lines later units still add:"
        grep '^>' <(diff "$tree_dir/$f" "$snap/$f" 2> /dev/null) || echo "  (none)"
    done
}

cmd_check() {
    (($# > 0)) || fail "check needs packages, e.g. ./internal/core/protoarray/..."
    local bad=0
    checkout_tree "$(git write-tree)"
    cd "$tree_dir"
    go vet "$@" || bad=1
    go test -count=1 "$@" > "$state/test.log" 2>&1 || bad=1
    grep -aE '^(ok|FAIL|--- FAIL)' "$state/test.log" | tail -20 || true
    if ((bad == 0)); then echo "=== check: PASS"; else echo "=== check: FAIL"; fi
    return "$bad"
}

# baseline and verify run the repo's own recipes and record the result per tree, so the same tree
# is never judged twice.
cmd_verify() {
    local mode=$1 tree result failed="" unformatted note=""
    if [[ $mode == baseline ]]; then tree=$(change_tree); else tree=$(git write-tree); fi
    result=$state/results/$tree
    if [[ -f $result ]]; then
        cp "$result.log" "$state/test.log"
        failed=$(cat "$result")
        note="cached for tree $tree; "
    else
        checkout_tree "$tree"
        echo "=== checked out tree $tree in $tree_dir"
        cd "$tree_dir"
        # golangci-lint skips files behind build tags, so gofmt checks every Go file itself.
        if ! unformatted=$(git ls-files -z '*.go' | xargs -0 gofmt -l 2>&1); then
            unformatted=${unformatted:-gofmt failed}
        fi
        [[ -z $unformatted ]] || { echo "FAILED: gofmt"; echo "$unformatted"; failed+=" gofmt"; }
        make lint > "$state/lint.log" 2>&1 || { echo "FAILED: make lint"; tail -20 "$state/lint.log"; failed+=" lint"; }
        make test > "$state/test.log" 2>&1 || { echo "FAILED: make test"; failed+=" tests"; }
        grep -aE '^(--- FAIL|FAIL|panic)' "$state/test.log" | tail -20 || true
        failed=${failed:+FAIL:$failed}
        mkdir -p "$state/results"
        cp "$state/test.log" "$result.log"
        echo "$failed" > "$result"
    fi
    echo "=== $mode: ${failed:-PASS}, $(passed) tests passed (${note}log: $state/test.log)"
    [[ -z $failed ]]
}

cmd_union_check() {
    need_snapshot
    local f bad=0
    while read -r f; do
        if [[ ! -f $snap/$f ]]; then
            if git cat-file -e ":$f" 2> /dev/null; then echo "  deletion not staged: $f"; bad=1; fi
        elif ! git cat-file -e ":$f" 2> /dev/null; then
            echo "  not staged: $f"
            bad=1
        elif ! git show ":$f" | cmp -s - "$snap/$f"; then
            echo "  staged content differs from the snapshot: $f"
            bad=1
        fi
    done < "$state/files"
    if ((bad == 0)); then echo "the units add up to the original change"; else echo "the units are incomplete"; fi
    return "$bad"
}

case ${1:-} in
    snapshot) cmd_snapshot ;;
    drift) cmd_drift ;;
    stale) cmd_stale ;;
    stage) shift; cmd_stage "$@" ;;
    rm) git rm -q --cached -- "${2:?usage: split.sh rm <path>}"; echo "staged the removal of $2" ;;
    unstage) git reset -q -- "${2:?usage: split.sh unstage <path>}"; echo "unstaged $2" ;;
    lock) cmd_lock ;;
    base-version)
        out=$state/base-version/${2:?usage: split.sh base-version <path>}
        mkdir -p "$(dirname "$out")"
        git show "HEAD:$2" > "$out"
        echo "$out"
        ;;
    check) shift; cmd_check "$@" ;;
    baseline | verify) cmd_verify "$1" ;;
    union-check) cmd_union_check ;;
    *) echo "usage: split.sh {baseline|snapshot|drift|stale|stage|rm|unstage|lock|base-version|verify|check|union-check}" ;;
esac
