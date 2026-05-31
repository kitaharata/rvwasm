package cpu_test

import (
	"testing"

	"github.com/kitaharata/rvwasm/internal/cpu"
	"github.com/kitaharata/rvwasm/internal/mem"
)

func put16(p []byte, off int, v uint16) {
	p[off] = byte(v)
	p[off+1] = byte(v >> 8)
}

func TestCLDSPUsesStackOffsetNotRDBits(t *testing.T) {
	const (
		dram  = 0x80000000
		code  = dram
		stack = dram + 0x100
	)
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)

	prog := make([]byte, 2)
	put16(prog, 0, 0x6082) // c.ldsp ra, 0(sp)
	if err := bus.Load(code, prog); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(stack+0, 8, 0x1111222233334444); err != nil {
		t.Fatal(err)
	}
	// Old decoder accidentally included rd bit 0 in the immediate and read +32.
	if err := bus.Write(stack+32, 8, 0xfeedfacecafebeef); err != nil {
		t.Fatal(err)
	}

	h.Reset(code, 0)
	h.X[2] = stack
	h.Run(1)
	if got := h.X[1]; got != 0x1111222233334444 {
		t.Fatalf("c.ldsp ra,0(sp) loaded %#x, want stack+0 value", got)
	}
}

func TestCLWSPUsesStackOffsetNotRDBits(t *testing.T) {
	const (
		dram  = 0x80000000
		code  = dram
		stack = dram + 0x100
	)
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)

	prog := make([]byte, 2)
	put16(prog, 0, 0x4082) // c.lwsp ra, 0(sp)
	if err := bus.Load(code, prog); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(stack+0, 4, 0x80000001); err != nil {
		t.Fatal(err)
	}
	// Old decoder accidentally included rd bit 0 in the immediate and read +32.
	if err := bus.Write(stack+32, 4, 0x12345678); err != nil {
		t.Fatal(err)
	}

	h.Reset(code, 0)
	h.X[2] = stack
	h.Run(1)
	if got := h.X[1]; got != 0xffffffff80000001 {
		t.Fatalf("c.lwsp ra,0(sp) loaded %#x, want sign-extended stack+0 value", got)
	}
}

func TestCLDSPHighOffsetBits(t *testing.T) {
	const (
		dram  = 0x80000000
		code  = dram
		stack = dram + 0x100
	)
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)

	prog := make([]byte, 2)
	put16(prog, 0, 0x6086) // c.ldsp ra, 64(sp)
	if err := bus.Load(code, prog); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(stack+64, 8, 0x5555666677778888); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(stack+96, 8, 0x9999aaaabbbbcccc); err != nil {
		t.Fatal(err)
	}

	h.Reset(code, 0)
	h.X[2] = stack
	h.Run(1)
	if got := h.X[1]; got != 0x5555666677778888 {
		t.Fatalf("c.ldsp ra,64(sp) loaded %#x, want stack+64 value", got)
	}
}
