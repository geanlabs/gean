# Gean architecture

Gean is one Go module organised by protocol. Lean Ethereum lives under `lean/`.
The Ethereum Beacon Chain will live under `beacon/`, laid out as one protocol
with many forks. Mechanics shared by both live in the root `internal/` and
contain no protocol types. One `gean` binary runs one protocol per process.

The [README](../README.md#architecture-overview) maps every current package.
This document states the layout, the rules that keep it clean, and the work
still to do.

## Layout

```text
gean/
├── cmd/
│   ├── gean/                     # `gean lean` today; `gean beacon` and `gean validator` later
│   └── keygen/                   # Lean XMSS keys
│
├── lean/                         # ═══ Lean Ethereum ═══
│   ├── consensus/                # Spec rules; pure
│   │   ├── types/  genesis/  statetransition/  forkchoice/
│   ├── crypto/xmss/              # leanVM bindings and the Rust FFI crates
│   ├── node/                     # Builds, runs and stops the Lean node
│   ├── internal/                 # Lean node services, private to lean/
│   │   ├── chain/                #   OWNER: ordered chain loop: ticks, import, head, pending blocks
│   │   ├── network/              #   OWNER: network loop: libp2p, topics, request/response, peers
│   │   │   └── syncer/           #     Range, by-root and checkpoint sync
│   │   ├── store/                #   Lean schema on top of internal/storage
│   │   ├── validator/            #   Attestation production, proposer selection, proposal reservation
│   │   ├── aggregation/          #   Aggregator role, sessions and proof results
│   │   ├── prover/               #   One proof at a time, proposals first
│   │   ├── blockbuilder/
│   │   └── api/                  #   /lean/v0 and the Hive test driver
│   └── spectests/                # leanSpec fixtures
│
├── beacon/                       # ═══ Ethereum Beacon Chain ═══ (README files only for now)
│   ├── consensus/                # Spec rules for every fork; pure
│   ├── crypto/                   # BLS, KZG
│   ├── node/                     # Builds and runs the Beacon node
│   ├── internal/                 # chain, processor, network/syncer, store, oppool,
│   │                             # execution, availability, api
│   ├── validator/                # Validator client, its own process
│   └── spectests/                # consensus-spec-tests runner
│
├── internal/                     # ═══ Shared mechanics: no Lean or Beacon types ═══
│   ├── storage/                  # KV contract, Pebble and memory backends
│   ├── lifecycle/                # Starting, cancelling and joining goroutines
│   ├── logger/
│   └── metrics/
│
├── docs/
└── tools/archcheck/              # Source-level boundary checks
```

Both protocol trees have the same shape: pure `consensus/`, `crypto/`, `node/`
as the composition root, private `internal/` services and `spectests/`. A
developer who knows one side can find their way around the other.

`syncer` and `logger` are named so they do not shadow the standard library's
`sync` and `log`.

## Boundary rules

1. `beacon/` and `lean/` never import each other.
2. The root `internal/` imports neither protocol. It handles bytes, keys, peers
   and timers, never blocks or states.
3. Each protocol's `consensus/` is pure: no wall-clock reads, storage I/O,
   network calls or goroutines. Beacon consensus may use `beacon/crypto` for
   specification-required signature and KZG checks. Lean consensus uses no cgo.
4. The Beacon validator client never imports `beacon/internal/`. It reaches a
   node only through the Beacon API, so it can run as a separate process.

`make check-architecture` enforces rules 1–4 and their transitive
dependencies. Go's `internal` rule keeps each protocol's services private to its
tree. New shared code goes into the root `internal/` only when both protocols
use it; until then it stays in the protocol that needs it.

## Runtime owners

Each protocol's node has two owners. The chain loop is the only writer of chain
state: it takes clock ticks, imports blocks, updates the head and runs duties.
The network loop owns peers and wire traffic: gossip, request/response and sync.
They exchange typed messages and share no mutable state. Beacon adds a bounded,
prioritized processor between them, so signature verification runs on workers
and never delays a tick.

Lean today keeps its existing ordered loop in `lean/internal/chain`. Verified
attestations and aggregation results still write the synchronized pools from
worker goroutines; changing that is a separate behaviour change with ordering
tests, not part of a package move.

## Beacon: forks and SSZ

Lean is not a Beacon fork. A fork is the same chain changing rules at a
scheduled point, with an upgrade from the old state to the new one; no
specification defines such a transition from the Beacon Chain to Lean, and the
two share no state shape, clock, signature scheme or gossip topics.

Beacon is one protocol with many forks. Each fork's containers are their own Go
structs:

- Containers no fork changes are defined once in `beacon/consensus/types`.
- Each fork has a package (`phase0` through `gloas`) defining only what it
  changed; a fork that leaves a container unchanged reuses the previous type.
- A small `State` interface exposes only the fields every fork has. Code uses a
  type switch on the concrete fork type only where the specification changes
  behaviour.
- Each fork's processing lives in its own package under
  `beacon/consensus/statetransition`. Upgrades follow the specification's
  `upgrade_to_<fork>`: the old fork's state in, a new value of the next fork's
  type out.

There is no single struct carrying every fork's fields: per-fork structs make
reading a field the fork does not have a compile error.

SSZ codecs are generated for both protocols with `sszgen`. Generated files are
never edited by hand; regenerate them and confirm a second run changes nothing.

## Current state and remaining work

The tree above is in place for every existing package. These pieces still
differ from the target:

- Some packages are kept whole as subpackages of their main owner, for example
  `lean/internal/chain/blockprocessor`, `lean/internal/chain/attestation`,
  `lean/internal/chain/pending` and `lean/internal/blockbuilder/attestationproof`.
  Extracting their ownership dissolves them.
- `lean/internal/store` still combines durable data, synchronized pools and the
  decoded-key cache. Split those owners before moving the schema behind
  `internal/storage`.
- Proposal signing and proving still run in the chain's proposal worker;
  `lean/internal/validator` holds the reservation and the decisions around it.
- `lean/internal/network` holds all networking. The shared libp2p transport is
  extracted into the root `internal/` when Beacon networking uses it too.
- `beacon/` has README files only. Before any Beacon code, pin the first
  supported network, consensus-spec release and fork range. The first milestone
  is a validating follower; the validator client follows.
- `gean beacon`, `gean lean` and `gean validator` subcommands, and recording
  protocol and network identity in each data directory, arrive with the Beacon
  follower, together with updates to the tooling that starts Gean today.

Every step preserves existing algorithms, encodings, validation order and tick
ordering, removes the old path in the same change, and keeps protocol, schema,
scheduling and dependency changes in separate changes from package moves.

## Documentation

Keep this document in step with the code. Add `docs/spec_deviations.md` when the
first intentional departure from a specification lands: one entry per
departure, naming the specification function it differs from, what Gean does
instead and why, updated in the same change.
