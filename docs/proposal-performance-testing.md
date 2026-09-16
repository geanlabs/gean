# Proposal performance integration test

The integration branch includes three changes: suppress duplicate proposal duties (#435), yield an aggregation session between proof attempts when a proposal is waiting (#433), and avoid the next expensive proposal stage when its parent has already changed (#438). AVX2 remains a separate prerequisite PR. Prover-thread configuration is not included.

These changes reduce avoidable work. They do not implement continuous aggregation, change attestation selection, or make an individual native proof faster. The three issues remain open pending their broader investigation and measured server results.

## Expected behavior

- A queued duty remains reserved through proving and acceptance. A matching pre-sign failure may release it; starting Sign makes it non-retryable. This is not persistent signing protection across restarts.
- Aggregation checks proposal priority before group preparation and immediately before native proving. It returns completed outputs and deletes only signatures represented by those outputs. Deferred groups retain their inputs. A proof already in flight cannot be canceled. There is a small race between checking priority and entering native code; this is cooperative yielding, not hard preemption.
- Proposal construction rechecks its parent before signing, before wrapping the proposer signature, and before final merging. Parent changes during the final merge are still handled by the existing acceptance check. A stopped post-sign attempt does not authorize signing another candidate for the duty.
- Proof statements, attestation selection, and aggregate publication behavior are unchanged.

## Measurements

`lean_proposal_stage_duration_seconds{stage="gate"}` measures gate acquisition, including failed waits. `stage="signature_proof"` measures the proposer Type-1 call; `stage="merge"` measures final Type-2 merging. The latter two are nested within the existing proposal proving duration, so do not add them to that total. Finite buckets extend through 64 seconds; use histogram sums/counts for means and account for the +Inf tail. Labels do not carry slot numbers or roots.

Aggregation deferrals appear in the existing group-skip metric with `reason="proposal_pending"` and the log `aggregation yielded to proposal`. Stale-stage exits use `skipped_stale` operation status and log the slot. A yielded session may still have successfully produced and published an aggregate.

## Comparison

Build the recorded revision using the normal Docker workflow; omit `--prover-threads`, which is not provided by this branch. Compare against a baseline with the same prover configuration, native revision, CPU allocation, topology, and workload. An earlier eight-thread run is not a controlled baseline for this branch's automatic thread setting.

Check duplicate starts, actual duties, completed/accepted proposals, stage time distributions, useful votes included, vote age, head-to-justified and justified-to-finalized gaps, CPU and RSS. A shorter proof-time mean caused by more empty blocks is not success. Higher proposal priority may delay aggregation; finality and useful coverage must be checked for that tradeoff. Also inspect sample counts: early exits mean fewer attempts reach later stages.

Use local deterministic tests for lifecycle and stage boundaries. Use a controlled long devnet run to evaluate net performance; do not treat unit test timing as a native prover benchmark or assume a speedup until measured.
