# beacon/internal/store

Beacon schema on top of the shared `internal/storage` KV backends: hot and cold
data, state storage and pruning. Records protocol and network identity in the
data directory and rejects a mismatch.
