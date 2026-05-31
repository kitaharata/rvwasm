package cpu

func (h *Hart) execSystem(inst uint32, rd uint64, rs1 uint64, funct3 uint32) {
	if funct3 == 0 {
		switch inst {
		case 0x00000073: // ECALL
			h.observeEcall()
			if h.Mode == PrivS && h.SBIShim && h.SBIHandler != nil {
				if handled, errorCode, value := h.SBIHandler(h, h.LastEcallExt, h.LastEcallFunc, h.LastEcallArgs); handled {
					h.X[10] = uint64(errorCode)
					h.X[11] = value
					h.traceSBIShim(errorCode, value)
					return
				}
			}
			switch h.Mode {
			case PrivU:
				h.raiseException(ExcEcallU, 0)
			case PrivS:
				h.raiseException(ExcEcallS, 0)
			default:
				h.raiseException(ExcEcallM, 0)
			}
		case 0x00100073: // EBREAK
			h.raiseException(ExcBreakpoint, 0)
		case 0x30200073: // MRET
			if h.Mode != PrivM {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			h.mret()
		case 0x10200073: // SRET
			if h.Mode < PrivS {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			if h.Mode == PrivS && h.CSR[CSR_MSTATUS]&MSTATUS_TSR != 0 {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			h.sret()
		case 0x10500073: // WFI
			if h.Mode != PrivM && h.CSR[CSR_MSTATUS]&MSTATUS_TW != 0 {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			h.Waiting = true
			// Cooperative wait. The scheduler keeps ticking devices and wakes on interrupt.
		default:
			// SFENCE.VMA has rs1/rs2 fields; match by funct7/opcode shape.
			if inst&0xfe007fff == 0x12000073 {
				if h.Mode < PrivS {
					h.raiseException(ExcIllegalInstruction, uint64(inst))
					return
				}
				if h.Mode == PrivS && h.CSR[CSR_MSTATUS]&MSTATUS_TVM != 0 {
					h.raiseException(ExcIllegalInstruction, uint64(inst))
				}
				return
			}
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
		return
	}

	csr := uint16(inst >> 20)
	if h.Mode == PrivS && csr == CSR_SATP && h.CSR[CSR_MSTATUS]&MSTATUS_TVM != 0 {
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	if !h.csrExists(csr) || !h.csrAllowed(csr, true) || !h.csrCounterAllowed(csr) {
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	old := h.csrRead(csr)
	h.traceCSR("read", csr, old, 0, false)
	zimm := uint64(rs1)
	var write bool = true
	var nv uint64
	switch funct3 {
	case 1: // CSRRW
		nv = h.X[rs1]
	case 2: // CSRRS
		nv = old | h.X[rs1]
		write = rs1 != 0
	case 3: // CSRRC
		nv = old &^ h.X[rs1]
		write = rs1 != 0
	case 5: // CSRRWI
		nv = zimm
	case 6: // CSRRSI
		nv = old | zimm
		write = zimm != 0
	case 7: // CSRRCI
		nv = old &^ zimm
		write = zimm != 0
	default:
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	if write && !h.csrAllowed(csr, false) {
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	if rd != 0 {
		h.X[rd] = old
	}
	if write {
		h.csrWrite(csr, nv)
		h.traceCSR("write", csr, old, nv, true)
	}
}

func (h *Hart) csrAllowed(addr uint16, readOnlyCheck bool) bool {
	req := PrivMode((addr >> 8) & 0x3)
	if h.Mode < req {
		return false
	}
	if !readOnlyCheck && ((addr>>10)&0x3) == 0x3 {
		return false
	}
	return true
}
