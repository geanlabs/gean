# zk: proving gean's state transition

A zkVM proof here shows one claim: applying unsigned block B to pre-state S
succeeds and yields post-state root R. The guest commits 96 bytes: the
pre-state root, the block root and the post-state root, so a proof for block
i+1 chains onto block i. Signatures and the block's leanVM proof are not
covered; the node checks them on import.

The guests run `zk/stf`, a Rust port of `internal/statetransition` with the
same functions, check order and errors, and SSZ types derived from the same
fields and limits. The Go transition stays the reference: every guest result
is compared with it.

| Path | What |
|---|---|
| `stf/` | The Rust port (`gean_stf::apply`), with SSZ from `ethereum_ssz` and `tree_hash` derives |
| `ethereum_hashing/` | Replaces the crates.io `ethereum_hashing`, which needs `ring` on every target but x86_64; hashes through `sha2` |
| `<zkvm>/guest/` | Reads one input, runs `apply`, commits the 96 bytes; a rejected input panics before committing |
| `<zkvm>/host/` | Executes, proves and verifies one guest on that zkVM |
| `risc0/methods/` | Builds the RISC Zero guest and writes out its binary |

Each guest replaces `sha2` with its zkVM's SHA-256 precompile through
`[patch.crates-io]`; OpenVM has no such fork, so its guest turns on
`ethereum_hashing`'s `openvm` feature instead.

## Equivalence with Go

`make zk-stf-test` writes the vectors (`cmd/stfprove vectors`) and runs them
through the port. The vectors are the generated chains and reject cases of
`internal/zkstf/zkvectors`, plus seeded mutations of votes, state fields and
input bytes; each records what the Go transition does: its public values, or
the kind of error (`zkstf.ErrorKind`). The port must reach the same outcome on
every one.

Two inputs are known to differ, and the port rejects both, as leanSpec's typed
SSZ would: a crafted pre-state whose `JustifiedSlots` the transition extends
past its 2^18-bit limit, which fastssz still hashes, and an empty list encoded
as a 4-byte zero offset, which fastssz decodes.

## Running a zkVM

Each zkVM needs its guest toolchain: `sp1up` (SP1 v6.8.1), `rzup` (RISC Zero
3.0.6), `cargo-zisk toolchain install` (ZisK v1.3.0-alpha) and
`cargo openvm toolchain install` (OpenVM v2.x.0-preview.2).

```sh
make zk-guest ZKVM=sp1     # zk/out/sp1/stf.elf
make zk-host ZKVM=sp1      # zk/sp1/host/target/release/sp1host
make zk-exec ZKVM=sp1      # every vector on the zkVM, compared with Go
make zk-prove ZKVM=sp1 INPUT=zk/out/vectors/0000.gstf
```

Every host speaks one command line: `execute <elf> <input>`,
`prove <elf> <input> <proof>` and `verify <elf> <proof>`. It exits 3 when the
guest fails and 4 when a proof is invalid. `prove` executes first and refuses a
failing input, then verifies its own proof; `verify` checks the proof against
the program's key and requires a successful exit. `cmd/stfprove` drives the
hosts and compares the proven public values with Go.

- **SP1**: core proof. On 31 GB of RAM, run one core-shard worker
  (`SP1_WORKER_NUM_CORE_WORKERS=1 SP1_WORKER_CORE_BUFFER_SIZE=1`).
- **RISC Zero**: composite receipt; the key is the image id.
- **ZisK**: VADCOP proof. Needs the proving key (`ZISK_PROVING_KEY`, default
  `~/.zisk/provingKey`); `verify` pins the program key to the ELF and the
  recursion key to the proving key's `vadcop_final` key or `ZISK_SETUP_VK`.
- **OpenVM**: app proof, checked against the ELF's execution commitment. The
  guest reveals its 96 bytes zero-padded to the VM's 128 public-value bytes.
