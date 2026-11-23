# Finalization debugging

## Checking progress

```bash
grep "Latest Finalized" gean_0.log | tail -20
grep "finalized advanced" gean_*.log | tail -20
```

Healthy: the finalized slot advances every few slots while the head advances. Stalled: it stays
put for 50+ slots while the head keeps moving.

Justification needs two-thirds of the validators; see
[the devnet-log-review skill](../SKILL.md#common-investigations).

## Common causes

### Too few validators voting on one fork

Validators on another fork, crashed or paused don't count toward the canonical chain's votes. If
the rest fall below two-thirds, nothing justifies. Check which nodes are running and whether
their heads agree ([FORK_ANALYSIS.md](FORK_ANALYSIS.md#reorgs-and-diverging-heads)).

### Attestations not reaching a block

Votes count only once an aggregator proves them and a proposer includes the proof.

```bash
grep "proposed block" gean_*.log | grep -o "attestations=[0-9]*" | sort | uniq -c   # 0 means no proofs
grep "aggregate:" gean_*.log | tail -10                     # proofs the aggregator built
grep "aggregation skipped\|hit budget\|overran budget" gean_*.log
grep "aggregated attestation verification failed" gean_*.log
grep "dropped .* gossip attestations" gean_*.log            # the verify queue overflowed
```

### Justification chain broken

A slot is justifiable only if its distance from the finalized slot is at most 5, a perfect square
(9, 16, 25…) or a pronic number (6, 12, 20, 30…); see `SlotIsJustifiableAfter` in
`internal/lean/transition/justifiable.go`. Missing blocks or attestations at those slots stall
finality.

```bash
grep "Latest Justified" gean_0.log | tail -30
grep "unknown to fork choice" gean_*.log    # the justified root is missing: the head can't advance
```

See [FORK_ANALYSIS.md](FORK_ANALYSIS.md) for forks and
[ERROR_CLASSIFICATION.md](ERROR_CLASSIFICATION.md) for error lines.
