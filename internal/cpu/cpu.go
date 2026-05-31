package cpu

import (
	"fmt"
	"github.com/kitaharata/rvwasm/internal/mem"
)

type PrivMode uint8

const (
	PrivU PrivMode = 0
	PrivS PrivMode = 1
	PrivM PrivMode = 3
)

type SBIHandler func(h *Hart, ext, fn uint64, args [6]uint64) (handled bool, errorCode int64, value uint64)

type Hart struct {
	X                 [32]uint64
	PC                uint64
	InstPC            uint64
	Mode              PrivMode
	Bus               *mem.Bus
	CSR               map[uint16]uint64
	HartID            uint64
	Cycle             uint64
	Instret           uint64
	Time              uint64
	Running           bool
	Halted            bool
	Waiting           bool
	Reservation       uint64
	HasReservation    bool
	Trace             bool
	LastTrapCause     uint64
	LastTrapTval      uint64
	LastTrapInterrupt bool
	traceRing         []string
	tracePos          int
	traceCount        int
	LastError         error
	trapRaised        bool
	LastEcallMode     PrivMode
	LastEcallExt      uint64
	LastEcallFunc     uint64
	LastEcallArgs     [6]uint64
	LastEcallCycle    uint64
	LastSBIClass      string
	SBIBaseCount      uint64
	SBITimeCount      uint64
	SBIIPICount       uint64
	SBIRFenceCount    uint64
	SBIHSMCount       uint64
	SBISRSTCount      uint64
	SBILegacyCount    uint64
	SBIOtherCount     uint64
	SBIShim           bool
	SBIHandler        SBIHandler
	CSRTrace          bool
	csrRing           []string
	csrPos            int
	csrCount          int
	csrCounts         map[uint16]CSRAccessCount
	Profile           bool
	pcProfile         map[uint64]uint64
	PMPActive         bool
	PMPChecked        bool
}

func NewHart(bus *mem.Bus) *Hart {
	h := &Hart{Bus: bus, HartID: 0, Mode: PrivM, Reservation: ^uint64(0)}
	h.initCSR()
	return h
}

func (h *Hart) Reset(entry, dtb uint64) {
	h.X = [32]uint64{}
	h.PC = entry
	h.InstPC = entry
	h.Mode = PrivM
	h.Cycle, h.Instret, h.Time = 0, 0, 0
	h.Running, h.Halted, h.Waiting = false, false, false
	h.Reservation, h.HasReservation = ^uint64(0), false
	h.LastError = nil
	h.LastTrapCause, h.LastTrapTval, h.LastTrapInterrupt = 0, 0, false
	h.LastEcallMode, h.LastEcallExt, h.LastEcallFunc, h.LastEcallArgs, h.LastEcallCycle = 0, 0, 0, [6]uint64{}, 0
	h.LastSBIClass = ""
	h.SBIBaseCount, h.SBITimeCount, h.SBIIPICount, h.SBIRFenceCount, h.SBIHSMCount, h.SBISRSTCount, h.SBILegacyCount, h.SBIOtherCount = 0, 0, 0, 0, 0, 0, 0, 0
	h.trapRaised = false
	h.traceRing = nil
	h.tracePos, h.traceCount = 0, 0
	h.csrRing = nil
	h.csrPos, h.csrCount = 0, 0
	h.csrCounts = nil
	h.pcProfile = nil
	h.PMPActive = false
	h.PMPChecked = false
	h.initCSR()
	h.X[10] = h.HartID
	h.X[11] = dtb
}

func (h *Hart) Regs() [32]uint64 { return h.X }

func (h *Hart) Run(n int) int {
	ran := 0
	for ran < n && !h.Halted {
		if ok := h.Step(); !ok {
			break
		}
		ran++
	}
	return ran
}

func (h *Hart) Step() bool {
	if h.Halted {
		return false
	}
	h.trapRaised = false
	h.Bus.Tick(1)
	h.Time++
	h.Cycle++
	if h.checkInterrupt() {
		h.Waiting = false
		h.X[0] = 0
		return true
	}
	if h.Waiting {
		h.X[0] = 0
		return true
	}
	oldpc := h.PC
	h.InstPC = oldpc
	h.notePCProfile(oldpc)
	if oldpc&1 != 0 {
		h.raiseException(ExcInstructionMisaligned, oldpc)
		h.X[0] = 0
		return true
	}
	raw16, err := h.readVirtualFetch(h.PC, 2)
	if err != nil {
		h.raiseAccessError(accessFetch, h.PC, err)
		h.X[0] = 0
		return true
	}
	var raw uint32
	var bits int
	if raw16&3 != 3 {
		raw, bits = uint32(raw16), 16
		h.PC += 2
		h.execC(uint16(raw16))
	} else {
		raw32, err := h.readVirtualFetch(h.PC, 4)
		if err != nil {
			h.raiseAccessError(accessFetch, h.PC, err)
			h.X[0] = 0
			return true
		}
		raw, bits = uint32(raw32), 32
		h.PC += 4
		h.exec32(uint32(raw32))
	}
	h.X[0] = 0
	if h.trapRaised {
		return true
	}
	h.traceStep(oldpc, raw, bits)
	h.Instret++
	if h.LastError != nil {
		h.Halted = true
		return false
	}
	return true
}

func (h *Hart) fault(msg string, args ...any) {
	h.LastError = fmt.Errorf(msg, args...)
}
