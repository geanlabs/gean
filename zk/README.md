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

## Status

| zkVM | Execute | Prove |
|---|---|---|
| ZisK | emulator, all vectors match native | not yet |
| SP1 | not yet | not yet |
| OpenVM | not yet | not yet |

## Running on ZisK

Install ZisK (`ziskemu` comes with it), then:

```sh
make zk-guest                  # download TamaGo, build zk/out/stf-zisk.elf
make zk-haltcheck              # only a committed run may halt successfully
make zk-exec                   # every generated vector, compared with native
bin/stfprove replay --zkvm zisk --backend emu blocks <from> <to>   # blocks from a running node
```

Set `ZISKEMU` if `ziskemu` is not on `PATH`. Every `stfprove` command takes
`--zkvm native|zisk|sp1|openvm`; `native` runs the Go transition directly and
is what every zkVM result is compared against.

## Soundness rules

- Any failure halts without a proof: a rejected transition, a panic, a nil
  dereference, or `main` returning. On ZisK the failing halt is a reserved
  instruction, because ZisK's exit call ignores the exit code.
- The guest recomputes the pre-state and block roots itself; the post-state
  root is the block's state root, which the transition has checked.
- The ELF is built reproducibly (`-trimpath -buildvcs=false -buildid=`), so the
  program's verifying key is stable.
