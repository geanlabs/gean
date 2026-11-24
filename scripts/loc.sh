#!/usr/bin/env bash
# Counts lines of code under internal/, leaving out tests, blank lines and // comment lines:
# Go per top-level package, and the Rust glue. The consensus total leaves out core and the
# Rust glue, which serve any chain. With a git ref, that ref's counts are shown beside them,
# with the change. Prints a Markdown table.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# count prints "<package> <lines>" for the tree at $1, with the Rust glue as "rust".
count() {
    (
        cd "$1"
        find internal -name target -prune \
            -o \( -name '*.rs' -o -name '*.go' ! -name '*_test.go' \) -print0 |
            xargs -0 awk '
                FNR == 1 { split(FILENAME, p, "/"); pkg = FILENAME ~ /\.rs$/ ? "rust" : p[2] }
                /^[[:space:]]*$/ || /^[[:space:]]*\/\// { next }
                { n[pkg]++ }
                END { for (k in n) print k, n[k] }'
    )
}

counts=$(count . | sed 's/^/head /')
if (($#)); then
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    # A ref without internal/ counts as empty.
    if git cat-file -e "$1:internal" 2>/dev/null; then
        git archive "$1" internal | tar -x -C "$tmp"
        counts+=$'\n'$(count "$tmp" | sed 's/^/base /')
    fi
fi

printf '%s\n' "$counts" | awk -v compare="$#" '
    NF == 3 { n[$1, $2] += $3; pkgs[$2] }
    function row(name, b, h) {
        if (compare) return sprintf("| %s | %d | %d | %+d |", name, b, h, h - b)
        return sprintf("| %s | %d |", name, h)
    }
    END {
        if (compare) print "| Package | Base | Head | Change |\n|---|---:|---:|---:|"
        else print "| Package | Lines |\n|---|---:|"
        fflush()
        for (p in pkgs) {
            if (p == "core" || p == "rust") continue
            print row(p, n["base", p], n["head", p]) | "sort"
            tb += n["base", p]; th += n["head", p]
        }
        close("sort")
        print row("**consensus total**", tb, th)
        print row("core", n["base", "core"], n["head", "core"])
        print row("rust glue", n["base", "rust"], n["head", "rust"])
    }'
