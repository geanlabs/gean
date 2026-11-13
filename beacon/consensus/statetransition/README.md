# beacon/consensus/statetransition

Entry point (`StateTransition`, `ProcessSlots`), shared helpers (`helpers/`:
committees, balances, predicates) and one package per fork for what that fork's
block and epoch processing changed. Upgrades follow the specification's
`upgrade_to_<fork>`: old fork's state in, a new value of the next fork's type out.
