# Proposal performance test

Build the integration branch and omit `--prover-threads`. Compare runs with the same prover configuration, CPU allocation, topology, and workload.

Check for fewer duplicate attempts, less proposal waiting, and fewer discarded proposals. Compare useful votes included, vote age, finality gaps, CPU, and RSS: faster empty blocks are not an improvement.

`lean_proposal_stage_duration_seconds` separates:

- `gate`: prover acquisition time, including failed waits.
- `signature_proof`: proposer Type-1 proof time.
- `merge`: final Type-2 proof time.

The proof stages are included in the existing proposal proving total; do not count them twice. Buckets extend through 64 seconds; account for the `+Inf` tail and differing sample counts.

Look for `proposal_pending` aggregation skips and `skipped_stale` proposal operations. Completed aggregates survive a yield; deferred signatures remain available. Native proofs already running cannot be interrupted.

This experiment reduces duplicate and stale work. Individual proof speed and overall finality gains still need measurement.
