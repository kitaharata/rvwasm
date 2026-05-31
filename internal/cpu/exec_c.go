package cpu

func (h *Hart) execC(ins uint16) {
	q := ins & 3
	funct3 := (ins >> 13) & 7
	switch q {
	case 0:
		h.execC0(ins, funct3)
	case 1:
		h.execC1(ins, funct3)
	case 2:
		h.execC2(ins, funct3)
	default:
		h.raiseException(ExcIllegalInstruction, uint64(ins))
	}
}

func (h *Hart) execC0(x uint16, f uint16) {
	rd := cReg(x)
	rs1 := cReg2(x)
	rs2 := cReg(x)
	switch f {
	case 0: // C.ADDI4SPN
		imm := uint64(((x >> 7) & 0x30) | ((x >> 1) & 0x3c0) | ((x >> 4) & 0x4) | ((x >> 2) & 0x8))
		if imm == 0 {
			h.raiseException(ExcIllegalInstruction, uint64(x))
			return
		}
		h.X[rd] = h.X[2] + imm
	case 2: // C.LW
		imm := uint64(((x >> 4) & 0x4) | ((x >> 7) & 0x38) | ((x << 1) & 0x40))
		v := h.load(h.X[rs1]+imm, 4, true)
		if h.trapRaised {
			return
		}
		h.X[rd] = v
	case 3: // C.LD
		imm := uint64(((x >> 7) & 0x38) | ((x << 1) & 0xc0))
		v := h.load(h.X[rs1]+imm, 8, false)
		if h.trapRaised {
			return
		}
		h.X[rd] = v
	case 6: // C.SW
		imm := uint64(((x >> 4) & 0x4) | ((x >> 7) & 0x38) | ((x << 1) & 0x40))
		h.store(h.X[rs1]+imm, 4, h.X[rs2])
	case 7: // C.SD
		imm := uint64(((x >> 7) & 0x38) | ((x << 1) & 0xc0))
		h.store(h.X[rs1]+imm, 8, h.X[rs2])
	default:
		h.raiseException(ExcIllegalInstruction, uint64(x))
	}
}

func (h *Hart) execC1(x uint16, f uint16) {
	rd := uint64((x >> 7) & 0x1f)
	rs1 := rd
	imm6 := sext(uint64(((x>>2)&0x1f)|((x>>7)&0x20)), 6)
	switch f {
	case 0: // C.ADDI / C.NOP
		if rd != 0 {
			h.X[rd] += imm6
		}
	case 1: // C.ADDIW
		if rd == 0 {
			h.raiseException(ExcIllegalInstruction, uint64(x))
			return
		}
		h.X[rd] = sext32(uint32(h.X[rd] + imm6))
	case 2: // C.LI
		if rd != 0 {
			h.X[rd] = imm6
		}
	case 3: // C.ADDI16SP / C.LUI
		if rd == 2 {
			imm := uint64(((x >> 3) & 0x200) | ((x >> 2) & 0x10) | ((x << 1) & 0x40) | ((x << 4) & 0x180) | ((x << 3) & 0x20))
			simm := sext(imm, 10)
			if simm == 0 {
				h.raiseException(ExcIllegalInstruction, uint64(x))
				return
			}
			h.X[2] += simm
		} else if rd != 0 {
			imm := sext(uint64(((x>>2)&0x1f)|((x>>7)&0x20)), 6) << 12
			if imm == 0 {
				h.raiseException(ExcIllegalInstruction, uint64(x))
				return
			}
			h.X[rd] = imm
		} else {
			h.raiseException(ExcIllegalInstruction, uint64(x))
		}
	case 4:
		h.execCArith(x)
	case 5: // C.J
		h.PC = (h.PC - 2) + cJImm(x)
	case 6: // C.BEQZ
		rs := cReg2(x)
		if h.X[rs] == 0 {
			h.PC = (h.PC - 2) + cBImm(x)
		}
	case 7: // C.BNEZ
		rs := cReg2(x)
		if h.X[rs] != 0 {
			h.PC = (h.PC - 2) + cBImm(x)
		}
	default:
		_ = rs1
		h.raiseException(ExcIllegalInstruction, uint64(x))
	}
}

func (h *Hart) execCArith(x uint16) {
	op := (x >> 10) & 3
	rd := cReg2(x)
	shamt := uint64(((x >> 2) & 0x1f) | ((x >> 7) & 0x20))
	switch op {
	case 0: // C.SRLI
		h.X[rd] >>= shamt
	case 1: // C.SRAI
		h.X[rd] = uint64(int64(h.X[rd]) >> shamt)
	case 2: // C.ANDI
		imm := sext(uint64(((x>>2)&0x1f)|((x>>7)&0x20)), 6)
		h.X[rd] &= imm
	case 3:
		rs2 := cReg(x)
		switch ((x >> 5) & 3) | (((x >> 12) & 1) << 2) {
		case 0:
			h.X[rd] = h.X[rd] - h.X[rs2] // C.SUB
		case 1:
			h.X[rd] = h.X[rd] ^ h.X[rs2] // C.XOR
		case 2:
			h.X[rd] = h.X[rd] | h.X[rs2] // C.OR
		case 3:
			h.X[rd] = h.X[rd] & h.X[rs2] // C.AND
		case 4:
			h.X[rd] = sext32(uint32(h.X[rd] - h.X[rs2])) // C.SUBW
		case 5:
			h.X[rd] = sext32(uint32(h.X[rd] + h.X[rs2])) // C.ADDW
		default:
			h.raiseException(ExcIllegalInstruction, uint64(x))
		}
	}
}

func (h *Hart) execC2(x uint16, f uint16) {
	rd := uint64((x >> 7) & 0x1f)
	rs2 := uint64((x >> 2) & 0x1f)
	switch f {
	case 0: // C.SLLI
		shamt := uint64(((x >> 2) & 0x1f) | ((x >> 7) & 0x20))
		if rd == 0 {
			h.raiseException(ExcIllegalInstruction, uint64(x))
			return
		}
		h.X[rd] <<= shamt
	case 2: // C.LWSP
		if rd == 0 {
			h.raiseException(ExcIllegalInstruction, uint64(x))
			return
		}
		imm := uint64(((x >> 2) & 0x1c) | ((x >> 7) & 0x20) | ((x << 4) & 0xc0))
		v := h.load(h.X[2]+imm, 4, true)
		if h.trapRaised {
			return
		}
		h.X[rd] = v
	case 3: // C.LDSP
		if rd == 0 {
			h.raiseException(ExcIllegalInstruction, uint64(x))
			return
		}
		imm := uint64(((x >> 2) & 0x18) | ((x >> 7) & 0x20) | ((x << 4) & 0x1c0))
		v := h.load(h.X[2]+imm, 8, false)
		if h.trapRaised {
			return
		}
		h.X[rd] = v
	case 4:
		bit12 := (x >> 12) & 1
		if bit12 == 0 {
			if rs2 == 0 {
				if rd == 0 {
					h.raiseException(ExcIllegalInstruction, uint64(x))
					return
				}
				h.PC = h.X[rd] &^ uint64(1) // C.JR, alias of JALR x0,0(rs1)
			} else {
				if rd == 0 {
					h.raiseException(ExcIllegalInstruction, uint64(x))
					return
				}
				h.X[rd] = h.X[rs2] // C.MV
			}
		} else {
			if rd == 0 && rs2 == 0 {
				h.raiseException(ExcBreakpoint, 0) // C.EBREAK
			} else if rs2 == 0 {
				t := h.X[rd] &^ uint64(1)
				h.X[1] = h.PC
				h.PC = t // C.JALR
			} else {
				if rd == 0 {
					h.raiseException(ExcIllegalInstruction, uint64(x))
					return
				}
				h.X[rd] += h.X[rs2] // C.ADD
			}
		}
	case 6: // C.SWSP
		imm := uint64(((x >> 7) & 0x3c) | ((x >> 1) & 0xc0))
		h.store(h.X[2]+imm, 4, h.X[rs2])
	case 7: // C.SDSP
		imm := uint64(((x >> 7) & 0x38) | ((x >> 1) & 0x1c0))
		h.store(h.X[2]+imm, 8, h.X[rs2])
	default:
		h.raiseException(ExcIllegalInstruction, uint64(x))
	}
}

func cJImm(x uint16) uint64 {
	v := uint64(((x>>12)&1)<<11 |
		((x>>8)&1)<<10 |
		((x>>9)&3)<<8 |
		((x>>6)&1)<<7 |
		((x>>7)&1)<<6 |
		((x>>2)&1)<<5 |
		((x>>11)&1)<<4 |
		((x>>3)&7)<<1)
	return sext(v, 12)
}

func cBImm(x uint16) uint64 {
	v := uint64(((x>>12)&1)<<8 |
		((x>>5)&3)<<6 |
		((x>>2)&1)<<5 |
		((x>>10)&3)<<3 |
		((x>>3)&3)<<1)
	return sext(v, 9)
}
