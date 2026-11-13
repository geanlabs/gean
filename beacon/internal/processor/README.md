# beacon/internal/processor

Bounded work queues with a fixed priority order (blocks before attestations),
explicit drops when full, and verification workers that feed results to the
chain loop.
