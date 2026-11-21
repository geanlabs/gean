"""Compares a hive run's results with the tests gean is expected to fail.

Usage: check.py <results-root> <expected-failures-file>

Exits non-zero when a test fails that is not listed, when a listed test
passes or did not run, or when the run produced no results.
"""

import json
import sys
from pathlib import Path


def main(results_root: str, expected_file: str) -> int:
    expected = set()
    for line in Path(expected_file).read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            expected.add(line)

    results = {}
    for path in Path(results_root).glob("*.json"):
        suite = json.loads(path.read_text())
        # hive also writes hive.json, a run description with no tests, here.
        if "testCases" not in suite:
            continue
        for case in suite["testCases"].values():
            test = f"{suite['name']}: {case['name']}"
            results[test] = results.get(test, True) and case["summaryResult"]["pass"]
    if not results:
        print(f"no hive results under {results_root}")
        return 1

    problems = [f"unexpected failure: {t}" for t, ok in sorted(results.items()) if not ok and t not in expected]
    problems += [f"unexpected pass, remove it from {expected_file}: {t}" for t, ok in sorted(results.items()) if ok and t in expected]
    problems += [f"listed but did not run, remove it from {expected_file}: {t}" for t in sorted(expected - results.keys())]

    print(f"hive: {sum(results.values())} of {len(results)} tests passed, {len(expected)} expected failures")
    for problem in problems:
        print(problem)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(*sys.argv[1:]))
