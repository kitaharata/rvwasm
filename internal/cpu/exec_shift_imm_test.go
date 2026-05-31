package cpu_test

import (
	"testing"

	"github.com/kitaharata/rvwasm/internal/cpu"
	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestRV64ShiftImmediateAllowsShamtBit5(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(bus)
	h.Reset(0x80000000, 0)

	prog := make([]byte, 16)
	put32(prog, 0, 0x02051613)  // slli a2,a0,32 (funct7==1 because shamt[5] is set)
	put32(prog, 4, 0x02065693)  // srli a3,a2,32
	put32(prog, 8, 0x42065713)  // srai a4,a2,32
	put32(prog, 12, 0x00100073) // ebreak
	if err := bus.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.X[10] = 0x12345678
	for i := 0; i < 3; i++ {
		if !h.Step() {
			t.Fatalf("step %d failed: pc=%#x cause=%#x tval=%#x err=%v", i, h.PC, h.CSR[cpu.CSR_MCAUSE], h.CSR[cpu.CSR_MTVAL], h.LastError)
		}
	}
	if got := h.X[12]; got != 0x1234567800000000 {
		t.Fatalf("slli result=%#x, want 0x1234567800000000", got)
	}
	if got := h.X[13]; got != 0x12345678 {
		t.Fatalf("srli result=%#x, want 0x12345678", got)
	}
	if got := h.X[14]; got != 0x12345678 {
		t.Fatalf("srai result=%#x, want 0x12345678", got)
	}
}

func TestRV64ShiftImmediateRejectsReservedFunct6(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(bus)
	h.Reset(0x80000000, 0)

	prog := make([]byte, 4)
	put32(prog, 0, 0x04051613) // reserved SLLI encoding: imm[11:6] != 0
	if err := bus.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.X[10] = 1
	h.Step()
	if got := h.CSR[cpu.CSR_MCAUSE]; got != cpu.ExcIllegalInstruction {
		t.Fatalf("mcause=%#x, want illegal instruction", got)
	}
	if got := h.CSR[cpu.CSR_MTVAL]; got != 0x04051613 {
		t.Fatalf("mtval=%#x, want raw instruction", got)
	}
}
