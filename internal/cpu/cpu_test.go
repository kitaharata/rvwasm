package cpu_test

import (
	"strings"

	"github.com/kitaharata/rvwasm/internal/cpu"
	"github.com/kitaharata/rvwasm/internal/dev"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func put32(p []byte, off int, v uint32) {
	p[off] = byte(v)
	p[off+1] = byte(v >> 8)
	p[off+2] = byte(v >> 16)
	p[off+3] = byte(v >> 24)
}

func TestUARTSmoke(t *testing.T) {
	bus := mem.NewBus(0x80000000, 4096)
	var out []byte
	bus.AddDevice(dev.UARTBase, 0x100, dev.NewUART(func(b byte) { out = append(out, b) }, nil))
	h := cpu.NewHart(bus)
	prog := make([]byte, 16)
	put32(prog, 0, 0x10000537)  // lui a0,0x10000
	put32(prog, 4, 0x04800593)  // addi a1,zero,'H'
	put32(prog, 8, 0x00b50023)  // sb a1,0(a0)
	put32(prog, 12, 0x0000006f) // jal zero,0
	if err := bus.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(0x80000000, 0)
	h.Run(3)
	if string(out) != "H" {
		t.Fatalf("got %q", string(out))
	}
}

func put64(p []byte, off int, v uint64) {
	p[off] = byte(v)
	p[off+1] = byte(v >> 8)
	p[off+2] = byte(v >> 16)
	p[off+3] = byte(v >> 24)
	p[off+4] = byte(v >> 32)
	p[off+5] = byte(v >> 40)
	p[off+6] = byte(v >> 48)
	p[off+7] = byte(v >> 56)
}

func pte(pa uint64, flags uint64) uint64 {
	return ((pa >> 12) << 10) | flags
}

func TestSv39FetchLoadStore(t *testing.T) {
	const (
		dram = 0x80000000
		root = 0x80001000
		l1   = 0x80002000
		l0   = 0x80003000
		code = 0x80004000
		data = 0x80005000
	)
	bus := mem.NewBus(dram, 0x10000)
	h := cpu.NewHart(bus)

	// root[0] -> l1, l1[2] -> l0, then map VA 0x400000 and 0x401000.
	if err := bus.Write(root+0*8, 8, pte(l1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(l1+2*8, 8, pte(l0, 1)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(l0+0*8, 8, pte(code, 1|2|8)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(l0+1*8, 8, pte(data, 1|2|4)); err != nil {
		t.Fatal(err)
	}

	prog := make([]byte, 20)
	put32(prog, 0, 0x00401537)  // lui a0,0x401
	put32(prog, 4, 0x00053583)  // ld a1,0(a0)
	put32(prog, 8, 0x00158593)  // addi a1,a1,1
	put32(prog, 12, 0x00b53023) // sd a1,0(a0)
	put32(prog, 16, 0x0000006f) // jal zero,0
	if err := bus.Load(code, prog); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(data, 8, 41); err != nil {
		t.Fatal(err)
	}

	h.Reset(0x400000, 0)
	h.Mode = cpu.PrivS
	h.CSR[cpu.CSR_SATP] = (uint64(8) << 60) | (root >> 12)
	h.Run(4)
	got, err := bus.Read(data, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("data = %d, want 42; status pc=%#x cause=%#x tval=%#x err=%v", got, h.PC, h.CSR[cpu.CSR_SCAUSE], h.CSR[cpu.CSR_STVAL], h.LastError)
	}
}

func TestPMPRestrictsSModePhysicalAccess(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x4000)
	h := cpu.NewHart(bus)

	prog := make([]byte, 8)
	put32(prog, 0, 0x00053583) // ld a1,0(a0) -- outside PMP TOR top
	put32(prog, 4, 0x0000006f) // jal zero,0
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}

	h.Reset(dram, 0)
	h.X[10] = dram + 0x1000
	h.Mode = cpu.PrivS
	h.CSR[cpu.CSR_PMPADDR0] = (dram + 0x1000) >> 2
	h.CSR[cpu.CSR_PMPCFG0] = 1 | 4 | (1 << 3) // R|X|TOR
	h.Run(1)
	if h.Mode != cpu.PrivM {
		t.Fatalf("mode=%d, want M after PMP trap", h.Mode)
	}
	if h.CSR[cpu.CSR_MCAUSE] != cpu.ExcLoadAccessFault {
		t.Fatalf("mcause=%#x, want load access fault", h.CSR[cpu.CSR_MCAUSE])
	}
	if h.CSR[cpu.CSR_MTVAL] != dram+0x1000 {
		t.Fatalf("mtval=%#x, want faulting address", h.CSR[cpu.CSR_MTVAL])
	}
}

func TestTraceRingRecordsInstructions(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	h.Trace = true
	prog := make([]byte, 8)
	put32(prog, 0, 0x00100513) // addi a0,zero,1
	put32(prog, 4, 0x0000006f) // jal zero,0
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Trace = true
	h.Run(2)
	tr := h.TraceString(4)
	if tr == "" || !strings.Contains(tr, "pc=0000000080000000") {
		t.Fatalf("trace missing first pc: %q", tr)
	}
}

func TestExceptionEPCIsFaultingInstruction(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 12)
	put32(prog, 0, 0x00100513) // addi a0,zero,1
	put32(prog, 4, 0x00000000) // illegal instruction
	put32(prog, 8, 0x00200513) // should not be reported as epc
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Run(2)
	if got := h.CSR[cpu.CSR_MEPC]; got != dram+4 {
		t.Fatalf("mepc=%#x, want faulting instruction %#x", got, dram+4)
	}
	if got := h.CSR[cpu.CSR_MCAUSE]; got != cpu.ExcIllegalInstruction {
		t.Fatalf("mcause=%#x, want illegal instruction", got)
	}
}

func TestMRETLeavesMModeClearsMPRV(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	h.Reset(dram, 0)
	h.CSR[cpu.CSR_MEPC] = dram + 0x100
	h.CSR[cpu.CSR_MSTATUS] = cpu.MSTATUS_MPRV | cpu.MSTATUS_MPIE | (uint64(cpu.PrivS) << cpu.MSTATUS_MPP_SHIFT)
	// mret
	prog := make([]byte, 4)
	put32(prog, 0, 0x30200073)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Step()
	if h.Mode != cpu.PrivS {
		t.Fatalf("mode=%d, want S", h.Mode)
	}
	if h.CSR[cpu.CSR_MSTATUS]&cpu.MSTATUS_MPRV != 0 {
		t.Fatalf("MPRV still set after mret to S: mstatus=%#x", h.CSR[cpu.CSR_MSTATUS])
	}
	if h.PC != dram+0x100 {
		t.Fatalf("pc=%#x", h.PC)
	}
}

func TestSupervisorTrapBits(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 12)
	put32(prog, 0, 0x18000073) // sfence.vma
	put32(prog, 4, 0x10500073) // wfi
	put32(prog, 8, 0x10200073) // sret
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.CSR[cpu.CSR_MSTATUS] |= cpu.MSTATUS_TVM
	h.Step()
	if h.CSR[cpu.CSR_MEPC] != dram || h.CSR[cpu.CSR_MCAUSE] != cpu.ExcIllegalInstruction {
		t.Fatalf("sfence trap mepc=%#x cause=%#x", h.CSR[cpu.CSR_MEPC], h.CSR[cpu.CSR_MCAUSE])
	}
}

func TestLoadFaultDoesNotRetireOrWriteRD(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 12)
	put32(prog, 0, 0x07b00593) // addi a1,zero,123
	put32(prog, 4, 0x00053583) // ld a1,0(a0) where a0 is zero/outside bus
	put32(prog, 8, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Run(2)
	if got := h.Regs()[11]; got != 123 {
		t.Fatalf("a1 changed on faulting load: got %d", got)
	}
	if got := h.ReadCSR(cpu.CSR_MINSTRET); got != 1 {
		t.Fatalf("instret=%d, want only first instruction retired", got)
	}
	if got := h.CSR[cpu.CSR_MEPC]; got != dram+4 {
		t.Fatalf("mepc=%#x, want faulting load", got)
	}
}

func TestReadOnlyCSRWriteDoesNotWriteRD(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 8)
	put32(prog, 0, 0xf1101573) // csrrw a0,mvendorid,zero (write to RO CSR)
	put32(prog, 4, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.X[10] = 0xfeed
	h.Step()
	if got := h.Regs()[10]; got != 0xfeed {
		t.Fatalf("a0 changed before illegal CSR trap: %#x", got)
	}
	if got := h.CSR[cpu.CSR_MCAUSE]; got != cpu.ExcIllegalInstruction {
		t.Fatalf("mcause=%#x, want illegal instruction", got)
	}
}

func TestCounterenBlocksSupervisorTime(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 8)
	put32(prog, 0, 0xc0102573) // csrr a0,time == CSRRS a0,time,x0
	put32(prog, 4, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.CSR[cpu.CSR_MCOUNTEREN] &^= 1 << 1
	h.Step()
	if got := h.CSR[cpu.CSR_MCAUSE]; got != cpu.ExcIllegalInstruction {
		t.Fatalf("mcause=%#x, want illegal instruction", got)
	}
}

func TestEcallObservationForSBITrace(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 20)
	put32(prog, 0, 0x00300513)  // addi a0,zero,3
	put32(prog, 4, 0x00200813)  // addi a6,zero,2
	put32(prog, 8, 0x00100893)  // addi a7,zero,1
	put32(prog, 12, 0x00000073) // ecall
	put32(prog, 16, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.Run(4)
	if h.LastEcallMode != cpu.PrivS || h.LastEcallExt != 1 || h.LastEcallFunc != 2 || h.LastEcallArgs[0] != 3 {
		t.Fatalf("ecall observation mode=%d ext=%#x func=%#x a0=%#x", h.LastEcallMode, h.LastEcallExt, h.LastEcallFunc, h.LastEcallArgs[0])
	}
	if h.CSR[cpu.CSR_MCAUSE] != cpu.ExcEcallS {
		t.Fatalf("mcause=%#x, want S ecall", h.CSR[cpu.CSR_MCAUSE])
	}
}

func TestSupervisorEnvCfgCSR(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 12)
	// csrrw zero,senvcfg,a0 ; csrr a1,senvcfg ; loop
	put32(prog, 0, uint32(cpu.CSR_SENVCFG)<<20|10<<15|1<<12|0x73)
	put32(prog, 4, uint32(cpu.CSR_SENVCFG)<<20|2<<12|11<<7|0x73)
	put32(prog, 8, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.CSR[cpu.CSR_MENVCFG] = 0xff
	h.X[10] = 0x5a5
	h.Run(2)
	if h.LastTrapCause != 0 || h.CSR[cpu.CSR_MCAUSE] == cpu.ExcIllegalInstruction {
		t.Fatalf("unexpected trap cause=%#x mcause=%#x", h.LastTrapCause, h.CSR[cpu.CSR_MCAUSE])
	}
	if got := h.Regs()[11]; got != 0xa5 { // masked by MENVCFG low byte
		t.Fatalf("senvcfg read %#x, want %#x", got, uint64(0xa5))
	}
}

func TestSBIClassCounters(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 4)
	put32(prog, 0, 0x00000073) // ecall
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.X[17] = 0x48534d // SBI HSM extension
	h.X[16] = 0
	h.Step()
	if h.LastSBIClass != "HSM" || h.SBIHSMCount != 1 {
		t.Fatalf("SBI class=%q hsm=%d summary=%s", h.LastSBIClass, h.SBIHSMCount, h.SBIObservationString())
	}
}

func TestSupervisorSBIShimHandlesEcallWithoutTrap(t *testing.T) {
	const dram = 0x80000000
	bus := mem.NewBus(dram, 0x1000)
	h := cpu.NewHart(bus)
	prog := make([]byte, 8)
	put32(prog, 0, 0x00000073) // ecall
	put32(prog, 4, 0x0000006f)
	if err := bus.Load(dram, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(dram, 0)
	h.Mode = cpu.PrivS
	h.X[10] = 1234
	h.X[16] = 0
	h.X[17] = 0x54494d45 // TIME
	h.SBIShim = true
	called := false
	h.SBIHandler = func(_ *cpu.Hart, ext, fn uint64, args [6]uint64) (bool, int64, uint64) {
		called = true
		if ext != 0x54494d45 || fn != 0 || args[0] != 1234 {
			t.Fatalf("handler got ext=%#x fn=%#x args0=%#x", ext, fn, args[0])
		}
		return true, 0, 0x55
	}
	h.Step()
	if !called {
		t.Fatal("SBI handler was not called")
	}
	if h.CSR[cpu.CSR_MCAUSE] == cpu.ExcEcallS {
		t.Fatalf("shimmed ecall still trapped: mcause=%#x", h.CSR[cpu.CSR_MCAUSE])
	}
	regs := h.Regs()
	if regs[10] != 0 || regs[11] != 0x55 {
		t.Fatalf("SBI return a0=%#x a1=%#x", regs[10], regs[11])
	}
	if h.PC != dram+4 {
		t.Fatalf("pc=%#x, want next instruction", h.PC)
	}
}

func TestWFIEntersWaitingState(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(bus)
	_ = bus.Write(0x80000000, 4, 0x10500073) // WFI
	h.Reset(0x80000000, 0)
	if !h.Step() {
		t.Fatalf("step failed: %v", h.LastError)
	}
	if !h.Waiting {
		t.Fatalf("hart did not enter waiting state")
	}
	pc := h.PC
	if !h.Step() {
		t.Fatalf("waiting step failed: %v", h.LastError)
	}
	if h.PC != pc {
		t.Fatalf("waiting hart advanced PC: got %#x want %#x", h.PC, pc)
	}
}

func TestCSRTraceRecordsReadWrite(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	h := cpu.NewHart(b)
	prog := make([]byte, 8)
	inst := uint32(cpu.CSR_MSTATUS)<<20 | 2<<15 | 1<<12 | 1<<7 | 0x73 // csrrw x1,mstatus,x2
	put32(prog, 0, inst)
	put32(prog, 4, 0x0000006f)
	if err := b.Load(0x80000000, prog); err != nil {
		t.Fatal(err)
	}
	h.Reset(0x80000000, 0)
	h.SetCSRTrace(true)
	h.X[2] = 0x1888
	if !h.Step() {
		t.Fatal("step failed")
	}
	text := h.CSRTraceString(8)
	if !strings.Contains(text, "csr-read") || !strings.Contains(text, "csr-write") || !strings.Contains(text, "0x300") {
		t.Fatalf("missing CSR trace, got:\n%s", text)
	}
}
