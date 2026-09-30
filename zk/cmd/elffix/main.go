// Command elffix makes a TamaGo guest ELF loadable by a zkVM and checks its
// instruction set.
//
// A zkVM transpiles every word of every executable segment before running, so
// the ELF must contain nothing it cannot decode:
//
//   - The linker places the ELF header in the first executable segment. The
//     segment is trimmed to start at .text.
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
package main

import (
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

// failWords is each zkVM's failing halt, a single 32-bit instruction.
var failWords = map[string]uint32{
	// SYSTEM word that ZisK decodes as reserved: halt_with_error.
	"zisk": 0x00200073,
}

func main() {
	zkvm := flag.String("zkvm", "", "target zkVM: zisk")
	flag.Parse()
	if flag.NArg() != 2 {
		log.Fatal("usage: elffix -zkvm <zkvm> <in.elf> <out.elf>")
	}
	fail, ok := failWords[*zkvm]
	if !ok {
		log.Fatalf("unsupported zkvm %q", *zkvm)
	}
	if err := fix(flag.Arg(0), flag.Arg(1), fail); err != nil {
		log.Fatal(err)
	}
}

func fix(in, out string, fail uint32) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	f, err := elf.NewFile(bytesReaderAt(data))
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

// trimTextSegment moves the start of the executable segment holding .text
// forward to .text, dropping the ELF header and notes from what is loaded as
// code.
func trimTextSegment(f *elf.File, data []byte, text *elf.Section) error {
	for i, p := range f.Progs {
		if p.Type != elf.PT_LOAD || p.Flags&elf.PF_X == 0 {
			continue
		}
		if text.Addr < p.Vaddr || text.Addr >= p.Vaddr+p.Memsz {
			return fmt.Errorf("executable segment at %#x does not hold .text", p.Vaddr)
		}
		d := text.Addr - p.Vaddr
		if d == 0 {
			return nil
		}
		base := int(binary.LittleEndian.Uint64(data[32:])) + i*56 // e_phoff + i*sizeof(Elf64_Phdr)
		put := func(field int, v uint64) { binary.LittleEndian.PutUint64(data[base+field:], v) }
		put(8, p.Off+d)
		put(16, p.Vaddr+d)
		put(24, p.Paddr+d)
		put(32, p.Filesz-d)
		put(40, p.Memsz-d)
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

func symbolAt(syms []elf.Symbol, addr uint64) string {
	i := sort.Search(len(syms), func(i int) bool { return syms[i].Value > addr }) - 1
	if i < 0 {
		return "?"
	}
	return syms[i].Name
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, errors.New("read past end")
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, errors.New("short read")
	}
	return n, nil
}
