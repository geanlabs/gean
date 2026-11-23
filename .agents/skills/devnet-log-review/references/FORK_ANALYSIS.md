# Fork analysis

Blocks at the same slot can be compared with the slot and root greps in
[the devnet-log-review skill](../SKILL.md#common-investigations). To see the structure, trace
which parent each block builds on.

## Parents

gean fetches a missing parent by root and holds the child until it arrives.

```bash
grep "received block" gean_0.log | head -20                          # slot, proposer, root, parent
grep "block parent missing\|queueing missing block" gean_0.log       # waiting for a parent
grep "fetch exhausted" gean_0.log                                    # the parent never came; children dropped
```

## Rejected blocks

```bash
grep "block processing failed" gean_*.log          # the reason follows the root
grep "rejecting pending block\|rejecting pre-finalized block" gean_*.log   # wrong proposer, too far ahead, or below finality
```

[ERROR_CLASSIFICATION.md](ERROR_CLASSIFICATION.md) says whose fault a failed block is.

## Reorgs and diverging heads

```bash
grep "REORG" gean_*.log                            # depth, the new and old head, justified and finalized
grep "Head Block Root" gean_*.log | tail -8        # the head each node reports in CHAIN STATUS
```

Nodes reporting different head roots for long are on different forks; see
[FINALIZATION_DEBUG.md](FINALIZATION_DEBUG.md) for what that does to finality.
