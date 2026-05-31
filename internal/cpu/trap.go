package cpu

const (
	ExcInstructionMisaligned  uint64 = 0
	ExcInstructionAccessFault uint64 = 1
	ExcIllegalInstruction     uint64 = 2
	ExcBreakpoint             uint64 = 3
	ExcLoadMisaligned         uint64 = 4
	ExcLoadAccessFault        uint64 = 5
	ExcStoreMisaligned        uint64 = 6
	ExcStoreAccessFault       uint64 = 7
	ExcEcallU                 uint64 = 8
	ExcEcallS                 uint64 = 9
	ExcEcallM                 uint64 = 11
	ExcInstructionPageFault   uint64 = 12
	ExcLoadPageFault          uint64 = 13
	ExcStorePageFault         uint64 = 15
)

const interruptBit uint64 = 1 << 63

func (h *Hart) raiseException(cause, tval uint64) {
	h.trap(cause, tval, false)
}

func (h *Hart) trap(cause, tval uint64, interrupt bool) {
	h.trapRaised = true
	h.LastTrapCause, h.LastTrapTval, h.LastTrapInterrupt = cause, tval, interrupt
	code := cause
	if interrupt {
		code |= interruptBit
	}
	epc := h.PC
	if !interrupt {
		epc = h.InstPC
	}
	delegated := false
	if interrupt {
		delegated = ((h.CSR[CSR_MIDELEG] >> cause) & 1) != 0
	} else {
		delegated = ((h.CSR[CSR_MEDELEG] >> cause) & 1) != 0
	}
	if delegated && h.Mode <= PrivS {
		h.traceTrap(cause, tval, interrupt, PrivS)
		h.CSR[CSR_SEPC] = epc
		h.CSR[CSR_SCAUSE] = code
		h.CSR[CSR_STVAL] = tval
		st := h.CSR[CSR_MSTATUS]
		if st&MSTATUS_SIE != 0 {
			st |= MSTATUS_SPIE
		} else {
			st &^= MSTATUS_SPIE
		}
		st &^= MSTATUS_SIE
		if h.Mode == PrivS {
			st |= MSTATUS_SPP
		} else {
			st &^= MSTATUS_SPP
		}
		h.CSR[CSR_MSTATUS] = st
		h.Mode = PrivS
		h.PC = vectorBase(h.CSR[CSR_STVEC], cause, interrupt)
		return
	}
	h.traceTrap(cause, tval, interrupt, PrivM)
	h.CSR[CSR_MEPC] = epc
	h.CSR[CSR_MCAUSE] = code
	h.CSR[CSR_MTVAL] = tval
	st := h.CSR[CSR_MSTATUS]
	if st&MSTATUS_MIE != 0 {
		st |= MSTATUS_MPIE
	} else {
		st &^= MSTATUS_MPIE
	}
	st &^= MSTATUS_MIE
	st = (st &^ MSTATUS_MPP_MASK) | (uint64(h.Mode) << MSTATUS_MPP_SHIFT)
	h.CSR[CSR_MSTATUS] = st
	h.Mode = PrivM
	h.PC = vectorBase(h.CSR[CSR_MTVEC], cause, interrupt)
}

func vectorBase(tvec uint64, cause uint64, interrupt bool) uint64 {
	mode := tvec & 3
	base := tvec &^ uint64(3)
	if mode == 1 && interrupt {
		return base + 4*cause
	}
	return base
}

func (h *Hart) mret() {
	st := h.CSR[CSR_MSTATUS]
	mpp := PrivMode((st & MSTATUS_MPP_MASK) >> MSTATUS_MPP_SHIFT)
	if st&MSTATUS_MPIE != 0 {
		st |= MSTATUS_MIE
	} else {
		st &^= MSTATUS_MIE
	}
	st |= MSTATUS_MPIE
	st &^= MSTATUS_MPP_MASK
	if mpp != PrivM {
		st &^= MSTATUS_MPRV
	}
	h.CSR[CSR_MSTATUS] = st
	h.Mode = mpp
	h.PC = h.CSR[CSR_MEPC]
}

func (h *Hart) sret() {
	st := h.CSR[CSR_MSTATUS]
	spp := PrivU
	if st&MSTATUS_SPP != 0 {
		spp = PrivS
	}
	if st&MSTATUS_SPIE != 0 {
		st |= MSTATUS_SIE
	} else {
		st &^= MSTATUS_SIE
	}
	st |= MSTATUS_SPIE
	st &^= MSTATUS_SPP
	h.CSR[CSR_MSTATUS] = st
	h.Mode = spp
	h.PC = h.CSR[CSR_SEPC]
}

func (h *Hart) checkInterrupt() bool {
	pending := h.CSR[CSR_MIP] & h.CSR[CSR_MIE]
	if pending == 0 {
		return false
	}
	mideleg := h.CSR[CSR_MIDELEG]
	mstatus := h.CSR[CSR_MSTATUS]

	order := []uint64{11, 3, 7, 9, 1, 5}
	for _, cause := range order {
		bit := uint64(1) << cause
		if pending&bit == 0 {
			continue
		}
		delegated := mideleg&bit != 0
		if delegated {
			if h.Mode < PrivS || (h.Mode == PrivS && mstatus&MSTATUS_SIE != 0) {
				h.trap(cause, 0, true)
				return true
			}
		} else {
			if h.Mode < PrivM || (h.Mode == PrivM && mstatus&MSTATUS_MIE != 0) {
				h.trap(cause, 0, true)
				return true
			}
		}
	}
	return false
}
