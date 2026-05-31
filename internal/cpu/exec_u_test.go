package cpu_test

import (
	"testing"

	"github.com/kitaharata/rvwasm/internal/cpu"
	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestLUIUpperImmediateSignExtendsOnRV64(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(bus)
	h.Reset(0x80000000, 0)
	prog := make([]byte, 8)
	put32(prog, 0, 0xfffff2b7) // lui t0,0xfffff
	put32(prog, 4, 0x00100073) // ebreak
	if err := bus.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.Step()
	if h.X[5] != 0xfffffffffffff000 {
		t.Fatalf("t0=%#x, want sign-extended 0xfffffffffffff000", h.X[5])
	}
}

func TestAUIPCUpperImmediateSignExtendsOnRV64(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(bus)
	h.Reset(0x80000000, 0)
	prog := make([]byte, 8)
	put32(prog, 0, 0xfffff297) // auipc t0,0xfffff => pc-0x1000
	put32(prog, 4, 0x00100073) // ebreak
	if err := bus.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.Step()
	if h.X[5] != 0x7ffff000 {
		t.Fatalf("t0=%#x, want 0x7ffff000", h.X[5])
	}
}
