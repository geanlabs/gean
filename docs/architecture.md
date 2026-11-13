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
│   ├── gean/                     # runs the Lean node; protocol subcommands come with Beacon
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
│   │   ├── pools/                #   Bounded signatures and new/known attestation payloads
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
3. Each protocol's consensus rules are pure: types, state transition and fork
   choice do no wall-clock reads, storage I/O, network calls or goroutines.
   Beacon consensus may use `beacon/crypto` for specification-required signature
   and KZG checks. Lean consensus uses no cgo. One exception exists today:
   `lean/consensus/genesis` also loads `config.yaml` from disk
   (`LoadGenesisConfig`). That is configuration loading, not a consensus rule,
   and belongs with node startup.
4. The Beacon validator client never imports `beacon/internal/`. It reaches a
   node only through the Beacon API, so it can run as a separate process.

`make check-architecture` enforces the import side of rules 1–4, including
transitive dependencies, aliases, generated files, tests and build tags, and it
rejects cgo in Lean consensus and the shared `internal/`. It parses imports only:
it cannot see standard-library calls such as `time.Now()`, `os.ReadFile` or a
`go` statement, so review must check those parts of rule 3. Go's `internal`
rule keeps each protocol's services private to its tree. New shared code goes into the root `internal/` only when both protocols
use it; until then it stays in the protocol that needs it.

The live pools depend only on Lean types. The store cannot import the pools,
and aggregation cannot import the store, including through helper packages.
Node assembly creates one pool set and passes it to the chain; workers receive
the pools and snapshots they use rather than persistence access.

## Runtime owners

The target for each protocol is two owners. The chain loop is the only writer
of chain state: it takes clock ticks, imports blocks, updates the head and runs
duties. The network loop owns peers and wire traffic: gossip,
request/response and sync. The two exchange typed messages. Beacon adds a
bounded, prioritized processor between them, so signature verification runs on
workers and never delays a tick.

Lean does not yet match that target. Its access rules today, which every change
must preserve until a deliberate ownership change replaces them:

- **The dispatch loop in `lean/internal/chain` is the only writer** of the head,
  safe target, justified and finalized checkpoints, store time, fork choice and
  block import. State that comments mark as loop-only (for example the chain's
  fetch and aggregation bookkeeping and the store's state-pruning marks) is
  never touched elsewhere.
- **Other goroutines read the live store.** Request/response handlers in
  `lean/internal/network`, registered by `lean/node`, serve status and blocks
  from it; gossip validators, the sync driver and the HTTP API read it too.
  These reads are safe because the store's accessors go through the storage
  backend or take the store's own locks. Fork-choice readers off the loop use
  the node snapshot fork choice publishes after each head update.
- **Worker goroutines write the attestation pools.** Gossip-attestation
  verification inserts into `AttestationSignatures`; aggregate verification,
  recovery and the aggregation worker push into `NewPayloads`. Each pool
  guards itself with a mutex. Do not add pool fields or caches that bypass it.

The pools live in `lean/internal/pools`, independently of `ConsensusStore`.
The chain advances the persisted clock before each interval's promotion, and
prunes live pools after finalization's persistence pruning. Aggregation receives
the head state/root and pool snapshots explicitly. Its worker applies completed
results directly to the shared pools in the existing order.

Moving those reads and writes behind the chain loop is a scheduling change with
its own ordering tests, not part of a package move.

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
- `lean/internal/store` owns durable data, persistence indexes/caches and the
  decoded-key cache. The live pools have been extracted; separating native
  verification-cache ownership remains before further store changes.
- Proposal signing and proving still run in the chain's proposal worker;
  `lean/internal/validator` holds the reservation and the decisions around it.
- `lean/internal/network` holds all networking. The shared libp2p transport is
  extracted into the root `internal/` when Beacon networking uses it too.
- `lean/consensus/genesis` loads `config.yaml` from disk; that loading moves to
  node startup so Lean consensus holds rules only.
- `beacon/` has README files only. Before any Beacon code, pin the first
  supported network, consensus-spec release and fork range. The first milestone
  is a validating follower; the validator client follows. Beacon implementation
  waits until the Lean refactor and its stability validation are complete.
- `gean` has no subcommands yet: today it runs the Lean node directly from its
  flags. `gean beacon`, `gean lean` and `gean validator` subcommands, and recording
  protocol and network identity in each data directory, arrive with the Beacon
  follower, together with updates to the tooling that starts Gean today.

Every step preserves existing algorithms, encodings, validation order and tick
ordering, removes the old path in the same change, and keeps protocol, schema,
scheduling and dependency changes in separate changes from package moves.

## Success criteria

The layout works when:

- Components run independently of the whole node.
- Lean and Beacon rules remain separate.
- Dependencies and resource ownership are easy to follow.
- New developers can make focused changes.
- Correctness and measured performance hold up.

Evidence, gathered as each boundary lands: a focused consumer that runs a
component without the rest of the node; import checks over both protocol
graphs; an owner, bound, full-queue behaviour and cancellation path for every
shared resource and worker; representative tasks (change a fork-choice rule,
add a wire message, adjust a storage operation, add an API field) carried out
by someone who did not design the component; and existing fixtures, regression
checks and benchmarks compared on the same workload. Moving files alone shows
organisation, not reusable boundaries.

## Lean 4 verification (proposed, parked)

Lean 4 here is the programming language and theorem prover, not Lean Ethereum.
The proposal: write the most critical rules as executable Lean 4 definitions
with machine-checked proofs about them, while Go keeps networking, storage,
process lifecycle and native integration. It is parked until the consensus and
store boundaries above are extracted and the Lean implementation is validated;
nothing in the Go layout depends on it,
because consensus rules already sit in their own pure packages.

When it resumes:

1. State the contract first: the pinned protocol revision, the implementation
   it describes, valid inputs, errors and the theorem, reviewed separately from
   the proof.
2. Start with a pure rule that existing fixtures already exercise, such as the
   justifiable-slot rule or the justification and finalization tally, and
   compare the Lean 4 and Go results on the same fixtures and malformed inputs.
   Comparison is test evidence, not proof that Go matches.
3. Only then prototype a runtime boundary and measure it, and promote the
   checked implementation when the evidence supports it, removing the
   superseded Go path rather than keeping both.

Proof gates when adopted: a pinned toolchain; warning-free, kernel-checked
builds; no admitted proofs or unreviewed axioms; an audit of each claimed
theorem's assumptions; and maintainer review of theorem statements and the
definitions they depend on. A proof about a Lean 4 model does not prove the Go
code, the compiler, the FFI or the native cryptography.

## Documentation

Keep this document in step with the code. Add `docs/spec_deviations.md` when the
first intentional departure from a specification lands: one entry per
departure, naming the specification function it differs from, what Gean does
instead and why, updated in the same change.
