package cpu

func (h *Hart) load(addr uint64, size int, signed bool) uint64 {
	if size != 1 && addr%uint64(size) != 0 {
		h.raiseException(ExcLoadMisaligned, addr)
		return 0
	}
	v, err := h.readVirtual(addr, size, accessLoad)
	if err != nil {
		h.raiseAccessError(accessLoad, addr, err)
		return 0
	}
	if signed {
		return sext(v, uint(size*8))
	}
	return v
}

func (h *Hart) store(addr uint64, size int, val uint64) {
	if size != 1 && addr%uint64(size) != 0 {
		h.raiseException(ExcStoreMisaligned, addr)
		return
	}
	if err := h.writeVirtual(addr, size, val); err != nil {
		h.raiseAccessError(accessStore, addr, err)
		return
	}
}

func (h *Hart) exec32(inst uint32) {
	opcode := inst & 0x7f
	rd := uint64((inst >> 7) & 0x1f)
	funct3 := (inst >> 12) & 7
	rs1 := uint64((inst >> 15) & 0x1f)
	rs2 := uint64((inst >> 20) & 0x1f)
	funct7 := (inst >> 25) & 0x7f

	switch opcode {
	case 0x37: // LUI
		h.X[rd] = immU(inst)
	case 0x17: // AUIPC
		h.X[rd] = (h.PC - 4) + immU(inst)
	case 0x6f: // JAL
		h.X[rd] = h.PC
		h.PC = (h.PC - 4) + immJ(inst)
	case 0x67: // JALR
		if funct3 != 0 {
			h.raiseException(ExcIllegalInstruction, uint64(inst))
			return
		}
		t := (h.X[rs1] + immI(inst)) &^ uint64(1)
		h.X[rd] = h.PC
		h.PC = t
	case 0x63:
		a, b := h.X[rs1], h.X[rs2]
		take := false
		switch funct3 {
		case 0:
			take = a == b
		case 1:
			take = a != b
		case 4:
			take = s64(a) < s64(b)
		case 5:
			take = s64(a) >= s64(b)
		case 6:
			take = a < b
		case 7:
			take = a >= b
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
			return
		}
		if take {
			h.PC = (h.PC - 4) + immB(inst)
		}
	case 0x03:
		addr := h.X[rs1] + immI(inst)
		var v uint64
		switch funct3 {
		case 0:
			v = h.load(addr, 1, true)
		case 1:
			v = h.load(addr, 2, true)
		case 2:
			v = h.load(addr, 4, true)
		case 3:
			v = h.load(addr, 8, false)
		case 4:
			v = h.load(addr, 1, false)
		case 5:
			v = h.load(addr, 2, false)
		case 6:
			v = h.load(addr, 4, false)
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
			return
		}
		if h.trapRaised {
			return
		}
		h.X[rd] = v
	case 0x23:
		addr := h.X[rs1] + immS(inst)
		switch funct3 {
		case 0:
			h.store(addr, 1, h.X[rs2])
		case 1:
			h.store(addr, 2, h.X[rs2])
		case 2:
			h.store(addr, 4, h.X[rs2])
		case 3:
			h.store(addr, 8, h.X[rs2])
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x13:
		shamt := (inst >> 20) & 0x3f
		funct6 := (inst >> 26) & 0x3f
		switch funct3 {
		case 0:
			h.X[rd] = h.X[rs1] + immI(inst)
		case 2:
			if s64(h.X[rs1]) < s64(immI(inst)) {
				h.X[rd] = 1
			} else {
				h.X[rd] = 0
			}
		case 3:
			if h.X[rs1] < immI(inst) {
				h.X[rd] = 1
			} else {
				h.X[rd] = 0
			}
		case 4:
			h.X[rd] = h.X[rs1] ^ immI(inst)
		case 6:
			h.X[rd] = h.X[rs1] | immI(inst)
		case 7:
			h.X[rd] = h.X[rs1] & immI(inst)
		case 1:
			// RV64 shift-immediate instructions use a 6-bit shamt.  Bit 25 is
			// shamt[5], not part of the opcode subfunction, so SLLI x?,x?,32
			// has funct7==1 and is still legal.  Check imm[11:6] instead.
			if funct6 != 0 {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			h.X[rd] = h.X[rs1] << shamt
		case 5:
			if funct6 == 0 {
				h.X[rd] = h.X[rs1] >> shamt
			} else if funct6 == 0x10 {
				h.X[rd] = uint64(s64(h.X[rs1]) >> shamt)
			} else {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
			}
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x1b:
		shamt := (inst >> 20) & 0x1f
		switch funct3 {
		case 0:
			h.X[rd] = sext32(uint32(h.X[rs1] + immI(inst)))
		case 1:
			if funct7 != 0 {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
				return
			}
			h.X[rd] = sext32(uint32(h.X[rs1]) << shamt)
		case 5:
			if funct7 == 0 {
				h.X[rd] = sext32(uint32(h.X[rs1]) >> shamt)
			} else if funct7 == 0x20 {
				h.X[rd] = sext32(uint32(int32(uint32(h.X[rs1])) >> shamt))
			} else {
				h.raiseException(ExcIllegalInstruction, uint64(inst))
			}
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x33:
		h.execOP(rd, rs1, rs2, funct3, funct7, inst)
	case 0x3b:
		h.execOP32(rd, rs1, rs2, funct3, funct7, inst)
	case 0x0f:
		if funct3 != 0 && funct3 != 1 {
			h.raiseException(ExcIllegalInstruction, uint64(inst))
			return
		}
		// FENCE/FENCE.I are no-ops in this interpreter.
	case 0x73:
		h.execSystem(inst, rd, rs1, funct3)
	case 0x2f:
		h.execAMO(inst, rd, rs1, rs2, funct3, funct7)
	default:
		h.raiseException(ExcIllegalInstruction, uint64(inst))
	}
}

func (h *Hart) execOP(rd, rs1, rs2 uint64, funct3 uint32, funct7 uint32, inst uint32) {
	a, b := h.X[rs1], h.X[rs2]
	switch funct7 {
	case 0x00:
		switch funct3 {
		case 0:
			h.X[rd] = a + b
		case 1:
			h.X[rd] = a << (b & 0x3f)
		case 2:
			if s64(a) < s64(b) {
				h.X[rd] = 1
			} else {
				h.X[rd] = 0
			}
		case 3:
			if a < b {
				h.X[rd] = 1
			} else {
				h.X[rd] = 0
			}
		case 4:
			h.X[rd] = a ^ b
		case 5:
			h.X[rd] = a >> (b & 0x3f)
		case 6:
			h.X[rd] = a | b
		case 7:
			h.X[rd] = a & b
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x20:
		switch funct3 {
		case 0:
			h.X[rd] = a - b
		case 5:
			h.X[rd] = uint64(s64(a) >> (b & 0x3f))
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x01:
		h.execM(rd, a, b, funct3)
	default:
		h.raiseException(ExcIllegalInstruction, uint64(inst))
	}
}

func (h *Hart) execOP32(rd, rs1, rs2 uint64, funct3 uint32, funct7 uint32, inst uint32) {
	a, b := uint32(h.X[rs1]), uint32(h.X[rs2])
	sh := b & 0x1f
	switch funct7 {
	case 0x00:
		switch funct3 {
		case 0:
			h.X[rd] = sext32(a + b)
		case 1:
			h.X[rd] = sext32(a << sh)
		case 5:
			h.X[rd] = sext32(a >> sh)
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x20:
		switch funct3 {
		case 0:
			h.X[rd] = sext32(a - b)
		case 5:
			h.X[rd] = sext32(uint32(int32(a) >> sh))
		default:
			h.raiseException(ExcIllegalInstruction, uint64(inst))
		}
	case 0x01:
		h.execM32(rd, uint64(a), uint64(b), funct3)
	default:
		h.raiseException(ExcIllegalInstruction, uint64(inst))
	}
}
