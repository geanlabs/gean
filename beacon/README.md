# beacon

The Ethereum Beacon Chain, laid out as one protocol with many forks.

**Status:** not implemented. This tree reserves the layout agreed in
[docs/architecture.md](../docs/architecture.md); each folder's
README says what will live there. Before any code lands, pin the first supported
network, consensus-spec release and fork range.

Rules (enforced by `make check-architecture` once code exists):

- Nothing here imports `lean/`, and nothing in `lean/` imports this tree.
- Shared mechanics come only from the root `internal/` (storage, logging,
  metrics, lifecycle); that tree carries no protocol types.
- `consensus/` is pure: no I/O, wall-clock reads or goroutines.
- `validator/` reaches the node only through the Beacon API.

| Folder | Holds |
| --- | --- |
| `consensus/` | Spec rules for every fork |
| `crypto/` | BLS and KZG |
| `node/` | Builds and runs the Beacon node (`gean beacon`) |
| `internal/` | Node services, private to `beacon/` |
| `validator/` | Validator client (`gean validator`) |
| `spectests/` | consensus-spec-tests runner |
