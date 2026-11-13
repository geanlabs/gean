# beacon/consensus

Pure Beacon Chain rules for every supported fork: types, configuration, the fork
schedule, state transition, shuffling and fork choice. No I/O, wall-clock reads
or goroutines; specification-required signature and KZG checks may use
`beacon/crypto`.
