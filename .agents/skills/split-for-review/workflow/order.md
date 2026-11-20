<!-- Adapted from silver (github.com/gattaca-com/silver), MIT License, Copyright (c) 2026 Gattaca. -->

# Workflow: Order

*Goal:* sequence the units so that every intermediate commit both builds and behaves. Cite this module when an intermediate compiled but was silently broken at runtime, or when a unit had to be re-cut because it could not be placed anywhere in the sequence.

## ORDER/COMPILE_ORDER_FROM_DEPENDENCIES
*Ensures:* each unit's prerequisites are already in HEAD when it is staged, and the units that need judgement arrive when the reviewer has the context for them.

Topologically sort the units on the dependency edges found in `UNITS/DEPENDENCY_DIRECTION`. Where the sort leaves freedom, this is the order, by `UNITS/TAXONOMY` kind:

1. independent pieces — unrelated fixes and anything else the feature needs only as a finished fact, including ones found mid-split (an outdated dependency, a misnamed file);
2. contracts, then mechanical follow-through — a reviewer reading the behaviour change should already have accepted the type it is expressed in;
3. behaviour, cut along its data flow in data-flow order;
4. harness; docs anywhere.

If two units are mutually dependent, they are one unit. Do not break a cycle with a temporary shim that no later unit removes — a scaffold that exists only between two commits is a lie in the history and a real cost to whoever bisects through it.

*Failure modes:* a cycle is "solved" with a compatibility shim that survives into the final state; a version bump lands after the feature that needs it, so reverting the feature cannot keep the bump.

## ORDER/NO_BROKEN_MIDDLE
*Ensures:* an intermediate commit is not merely compilable but also correct, so the series is bisectable for real.

Compiling is the floor, not the bar. The specific trap: a unit that **deletes a consumer path before the unit that adds its replacement producer**. It compiles — nothing references the deleted path — and it passes unit tests that drive the consumer directly, while in a running system the fact simply stops being reported.

The rule: **the path that produces a fact and the path that consumes it change in the same unit, or the old path survives until the new producer lands.** When a consumer must be rewired ahead of its producer, keep both routes alive in the earlier unit — the new arm is dead code for one commit, which is honest and harmless — and delete the old route in the unit that switches the producer.

Test-suite green is not evidence here, because unit tests usually call the consumer directly and never notice that nothing calls it any more. Check by asking, for each fact the change reports: in this intermediate, who emits it, and who acts on it?

*Failure modes:* an intermediate commit compiles and tests green but stops advancing a sync window, updating a metric, or firing an event, because the producer arrives two commits later; a bisect lands on that commit and the bug is attributed to it.

## ORDER/NO_DEAD_ARM
*Ensures:* a unit does not carry code that nothing can reach yet.

`ORDER/NO_BROKEN_MIDDLE` forbids deleting the old route early. Its mirror image: do not add the *new* route early either. A consumer arm whose only producer arrives two units later is dead code in every intermediate commit — the reviewer cannot judge it, and a bisect through those commits exercises nothing.

When the field a routing arm reads is added earlier than the producer that fills it, split them: the earlier unit widens the contract and has its existing producers label themselves; the later unit adds the arm, the producer, and the deletion of the old route together. When two data-flow stages cannot compile apart without such code, they are one unit.

The exception is the one `NO_BROKEN_MIDDLE` requires: an old route kept alive alongside a new one for a single commit, deleted by the unit that switches the producer.

*Failure modes:* a `match origin { Live => … }` arm lands with nothing emitting `Live`, so its behaviour is unreviewable; a "setup" unit whose only caller is in the next unit fails `-D warnings`.
