# zk: gean's state transition as a zkVM guest

The guest proves one claim: applying unsigned block B to pre-state S succeeds
and yields post-state root R. It commits 96 bytes of public values: the
pre-state root, the block root and the post-state root. A proof for block i+1
chains onto block i because its pre-state root is block i's post-state root.
Signatures and the block's leanVM proof are not covered; the node verifies them
on import.

The guest is gean's own Go state transition (`internal/zkstf.Apply`), built
bare-metal with [TamaGo](https://github.com/usbarmory/tamago-go) for 64-bit
RISC-V. This is a separate Go module so TamaGo-only code never reaches gean's
normal build.

| Path | What |
|---|---|
| `goos/` | TamaGo runtime overlay (`GOOSPKG`): memory map, fake clock, deterministic RNG, failing exit |
| `zkio/` | Guest I/O per zkVM: `ReadInput`, `Commit`, `Succeed`, `Fail` |
| `guest/stf/` | The guest program |
| `guest/haltcheck/` | Termination-path test guest |
| `cmd/elffix/` | Post-link step: trims the ELF header from the code segment and replaces unreachable float, vector and padding words with the failing halt |
| `cmd/haltcheck/` | Runs the haltcheck guest and checks only a committed run succeeds |
| `toolchain/patches/` | TamaGo patches for RV64IM zkVMs: no atomics or fences, explicit nil checks |
| `sha256zk/` | Replaces `minio/sha256-simd` in guest builds so SSZ hashing uses each zkVM's SHA-256 precompile |
| `ziskhost/`, `sp1host/`, `openvmhost/` | Rust hosts that prove and verify a guest (opt-in, not part of gean's build) |

## Toolchain

`make zk-toolchain` downloads the pinned TamaGo release, applies
`toolchain/patches/` and rebuilds its compiler. The patches only change
tamago/riscv64 builds:

- SP1 has no A extension and no fences. Atomics become plain loads and stores
  (the guest is single-threaded with no interrupts) and are never inlined as
  AMO or LR/SC; the publication barrier is not a FENCE.
- SP1 reads and writes address zero like any other memory, so a nil
  dereference would not fault. Every nil check stays in the code as a branch
  to `runtime.panicmem`.

OpenVM has the same limits. Every zkVM guest is built with the patched
toolchain, so all three run the same code.

## SHA-256 precompiles

gean's SSZ hashing (fastssz) calls `github.com/minio/sha256-simd`. The zk
module replaces it with `sha256zk`, which compresses each 64-byte block with the
zkVM's precompile: ZisK's `sha256f` CSR call, SP1's `SHA_EXTEND` and
`SHA_COMPRESS` syscalls, and OpenVM's SHA-256 instruction. Outside a guest build
it compresses in plain Go. Public values are unchanged on every vector; the
first full-block vectors drop from about 6-7M to 4M cycles on SP1 and from
about 6-7M to 3.7-3.8M steps on ZisK.

## Status

| zkVM | Execute | Prove and verify |
|---|---|---|
| ZisK | `ziskemu`, all vectors match native | `ziskhost` (ZisK v1.3.0-alpha), untested |
| SP1 | `sp1host`, all vectors match native | `sp1host` (SP1 v6.8.1 core proof), untested |
| OpenVM | `openvmhost`, all vectors match native | `openvmhost` (OpenVM v2.x.0-preview.2 app proof), untested |

## Hosts

Each zkVM has a Rust host, built with `make zk-host ZKVM=<zkvm>`, that speaks
one command line: `execute`, `prove` and `verify`, exiting 3 when the guest
fails and 4 when a proof is invalid. `stfprove` finds it through `<ZKVM>HOST`
(`ZISKHOST`, `SP1HOST`, `OPENVMHOST`) or `<zkvm>host` on `PATH`; `--host`
overrides both. ZisK executes on `ziskemu` (`ZISKEMU`) and needs `ziskhost`
only to prove and verify.

```sh
make zk-host ZKVM=sp1          # zk/sp1host/target/release/sp1host
make zk-guest ZKVM=sp1         # zk/out/stf-sp1.elf
make zk-haltcheck ZKVM=sp1     # only a committed run may halt successfully
make zk-exec ZKVM=sp1          # every generated vector, compared with native
export SP1HOST=$PWD/zk/sp1host/target/release/sp1host
bin/stfprove prove --zkvm sp1 -i zk/out/vectors/empty/slot-001.bin -o empty-001.proof
bin/stfprove verify --zkvm sp1 empty-001.proof
bin/stfprove replay --zkvm sp1 blocks <from> <to>   # blocks from a running node
bin/stfprove zkvms             # what each zkVM can do here
```

Every `stfprove` command takes `--zkvm native|zisk|sp1|openvm`; `native` runs
the Go transition directly and is what every zkVM result is compared against.
`verify` checks the proof in the host, then that the proof file's verifying key
and public values match what the host verified.

- **ZisK**: `ziskhost` builds against the ZisK SDK, which needs ZisK's apt
  dependencies (`libomp-dev libgmp-dev nlohmann-json3-dev libsodium-dev
  libsecp256k1-dev libopenmpi-dev`). Proving needs the ZisK proving key
  (`ZISK_PROVING_KEY`, default `~/.zisk/provingKey`), and the program key only
  exists once `prove` has run the setup. A ZisK proof embeds its program key
  and its recursion key; `verify` pins both, the first to the ELF and the
  second to the proving key's `vadcop_final` key or `ZISK_SETUP_VK`.
- **SP1**: `prove` makes a core proof on the CPU. `verify` requires exit code 0.
- **OpenVM**: RV64 guests exist only on OpenVM's `v2.x.0-preview.2` git tag.
  `prove` makes an app proof; `verify` checks it against the app verifying key
  and the ELF's execution commitment, and OpenVM only accepts a final exit
  code of 0.

## Soundness rules

- Any failure halts without a proof: a rejected transition, a panic, a nil
  dereference, or `main` returning. On ZisK the failing halt is a reserved
  instruction, because ZisK's exit call ignores the exit code. On SP1 it is
  `unimp`, which aborts execution; SP1 can prove a halt with any exit code, so
  a failure never halts. On OpenVM it is `terminate(1)`, which no OpenVM proof
  accepts.
- OpenVM panics on a pointer with any of its upper 32 bits set, and Go
  sign-extends 32-bit constants from `0x8000_0000` up, so the OpenVM board keeps
  code, RAM and input below `0x8000_0000`.
- The guest recomputes the pre-state and block roots itself; the post-state
  root is the block's state root, which the transition has checked.
- The ELF is built reproducibly (`-trimpath -buildvcs=false -buildid=`), so the
  program's verifying key is stable.
