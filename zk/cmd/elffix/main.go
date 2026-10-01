// Command elffix makes a TamaGo guest ELF loadable by a zkVM and checks its
// instruction set.
//
// A zkVM transpiles every word of every executable segment before running, so
// the ELF must contain nothing it cannot decode:
//
//   - The linker places the ELF header in the first executable segment, and
//     .text can end in a partial word of zero padding. The segment is trimmed
//     to the whole words of .text.
//   - The zkVM targets have no floating-point or vector unit. The guest is
//     built with soft float, but a few runtime assembly routines still name
//     float registers, and the vector paths of the byte and random-number
//     routines are compiled in behind a runtime check of
//     internal/cpu.RISCV64.HasV. None of it runs in the guest: there is no
//     asynchronous preemption, no reflect call passes floats in registers,
//     and TamaGo never sets HasV. Each such instruction is replaced by the
//     zkVM's failing halt, so reaching one aborts the run and can never yield
//     a proof. A float or vector instruction anywhere else fails the build.
//   - The linker pads between functions with zero words, which no zkVM
//     decodes. Padding is never executed; it becomes the failing halt too.
//   - Compressed instructions are rejected outright; the guest is built
//     without them.
//   - SP1 and OpenVM implement RV64IM only: SP1 rejects atomics at load time
//     and aborts on FENCE or a CSR access. The patched toolchain emits none
//     of them, and any left in an SP1 or OpenVM guest fails the build.
package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
)

// unreachableFloat and unreachableVector list the functions whose float or
// vector instructions may be replaced by a failing halt.
var (
	unreachableFloat = map[string]bool{
		"runtime.asyncPreempt.abi0":       true,
		"runtime.spillArgs.abi0":          true,
		"runtime.unspillArgs.abi0":        true,
		"reflect.archFloat32ToReg.abi0":   true,
		"reflect.archFloat32FromReg.abi0": true,
	}
	unreachableVector = map[string]bool{
		"compare":                    true, // internal/bytealg
		"indexByteBig":               true, // internal/bytealg
		"runtime.memequal":           true,
		"internal/chacha8rand.block": true,
	}
)

// target is one zkVM's instruction policy.
type target struct {
	// fail is the zkVM's failing halt, a single 32-bit instruction.
	fail uint32
	// rv64im rejects atomics, fences and CSR accesses other than fail.
	rv64im bool
}

var targets = map[string]target{
	// SYSTEM word that ZisK decodes as reserved: halt_with_error.
	"zisk": {fail: 0x00200073},
	// UNIMP (csrrw x0, cycle, x0), which SP1 executes as an abort.
	"sp1": {fail: 0xc0001073, rv64im: true},
	// terminate(1) on OpenVM's custom-0 opcode: exit code 1 is not provable.
	// OpenVM would itself turn an atomic or CSR word into a failing
	// terminate and a FENCE into a no-op, but the guest is held to RV64IM
	// all the same.
	"openvm": {fail: 0x0010000b, rv64im: true},
}

func main() {
	zkvm := flag.String("zkvm", "", "target zkVM: zisk, sp1 or openvm")
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("usage: elffix -zkvm <zkvm> <in.elf> <out.elf>")
	}
	t, ok := targets[*zkvm]
	if !ok {
		log.Fatalf("unsupported zkvm %q", *zkvm)
	}
	if err := fix(flag.Arg(0), flag.Arg(1), t); err != nil {
		log.Fatal(err)
	}
}

func fix(in, out string, t target) error {
	fail := t.fail
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if f.Class != elf.ELFCLASS64 || f.Machine != elf.EM_RISCV || f.ByteOrder != binary.LittleEndian {
		return errors.New("not a little-endian 64-bit RISC-V ELF")
	}
	text := f.Section(".text")
	if text == nil {
		return errors.New("no .text section")
	}
	syms, err := f.Symbols()
	if err != nil {
		return fmt.Errorf("symbols: %w", err)
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i].Value < syms[j].Value })

	if err := trimTextSegment(f, data, text); err != nil {
		return err
	}

	patched := map[string]int{}
	padding := 0
	for off := uint64(0); off+4 <= text.Size; off += 4 {
		pos := text.Offset + off
		word := binary.LittleEndian.Uint32(data[pos:])
		addr := text.Addr + off
		switch {
		case word == 0:
			binary.LittleEndian.PutUint32(data[pos:], fail)
			padding++
		case word&3 != 3:
			return fmt.Errorf("compressed instruction %#08x at %#x (%s)", word, addr, symbolAt(syms, addr))
		case t.rv64im && word != fail && isAtomicFenceOrCSR(word):
			return fmt.Errorf("atomic, fence or CSR instruction %#08x at %#x in %s", word, addr, symbolAt(syms, addr))
		case isVector(word):
			sym := symbolAt(syms, addr)
			if !unreachableVector[sym] {
				return fmt.Errorf("vector instruction %#08x at %#x in %s", word, addr, sym)
			}
			binary.LittleEndian.PutUint32(data[pos:], fail)
			patched[sym]++
		case isFloat(word):
			sym := symbolAt(syms, addr)
			if !unreachableFloat[sym] {
				return fmt.Errorf("float instruction %#08x at %#x in %s", word, addr, sym)
			}
			binary.LittleEndian.PutUint32(data[pos:], fail)
			patched[sym]++
		}
	}
	fmt.Printf("replaced %d zero padding words with the failing halt\n", padding)
	for sym, n := range patched {
		fmt.Printf("replaced %d float or vector instructions in %s with the failing halt\n", n, sym)
	}
	return os.WriteFile(out, data, 0o755)
}

// trimTextSegment shrinks the executable segment to the whole words of .text:
// it drops the ELF header and notes before .text, and the partial word of
// zero padding the linker can leave at its end.
func trimTextSegment(f *elf.File, data []byte, text *elf.Section) error {
	for i, p := range f.Progs {
		if p.Type != elf.PT_LOAD || p.Flags&elf.PF_X == 0 {
			continue
		}
		end := text.Addr + text.Size
		if text.Addr < p.Vaddr || end != p.Vaddr+p.Filesz || p.Filesz != p.Memsz {
			return fmt.Errorf("executable segment at %#x is not the ELF header and notes followed by .text", p.Vaddr)
		}
		size := text.Size &^ 3
		for _, b := range data[text.Offset+size : text.Offset+text.Size] {
			if b != 0 {
				return errors.New(".text ends in a partial instruction")
			}
		}
		base := int(binary.LittleEndian.Uint64(data[32:])) + i*56 // e_phoff + i*sizeof(Elf64_Phdr)
		put := func(field int, v uint64) { binary.LittleEndian.PutUint64(data[base+field:], v) }
		put(8, text.Offset)
		put(16, text.Addr)
		put(24, p.Paddr+(text.Addr-p.Vaddr))
		put(32, size)
		put(40, size)
		return nil
	}
	return errors.New("no executable segment")
}

// isVector reports whether a 32-bit RISC-V word is a V-extension
// instruction: OP-V, or a LOAD-FP/STORE-FP major opcode with a vector width.
func isVector(word uint32) bool {
	switch word & 0x7f {
	case 0x57:
		return true
	case 0x07, 0x27:
		switch (word >> 12) & 7 {
		case 0, 5, 6, 7:
			return true
		}
	}
	return false
}

// isFloat reports whether a 32-bit RISC-V word is an F/D-extension
// instruction: float loads/stores, fused multiply-adds, or OP-FP.
func isFloat(word uint32) bool {
	switch word & 0x7f {
	case 0x07, 0x27, 0x43, 0x47, 0x4b, 0x4f, 0x53:
		return true
	}
	return false
}

// isAtomicFenceOrCSR reports whether a 32-bit RISC-V word is outside RV64IM
// in a way a float check does not catch: an A-extension instruction (AMO
// major opcode), a FENCE, or a SYSTEM instruction that accesses a CSR.
func isAtomicFenceOrCSR(word uint32) bool {
	switch word & 0x7f {
	case 0x2f, 0x0f:
		return true
	case 0x73:
		return (word>>12)&7 != 0
	}
	return false
}

func symbolAt(syms []elf.Symbol, addr uint64) string {
	i := sort.Search(len(syms), func(i int) bool { return syms[i].Value > addr }) - 1
	if i < 0 {
		return "?"
	}
	return syms[i].Name
}
