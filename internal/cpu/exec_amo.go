package cpu

func (h *Hart) execAMO(inst uint32, rd, rs1, rs2 uint64, funct3 uint32, funct7 uint32) {
	aqrlFunct5 := (inst >> 27) & 0x1f
	addr := h.X[rs1]
	width := 4
	if funct3 == 3 {
		width = 8
	} else if funct3 != 2 {
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	if width == 4 && addr%4 != 0 {
		h.raiseException(ExcLoadMisaligned, addr)
		return
	}
	if width == 8 && addr%8 != 0 {
		h.raiseException(ExcLoadMisaligned, addr)
		return
	}

	switch aqrlFunct5 {
	case 0x02: // LR
		if rs2 != 0 {
			h.raiseException(ExcIllegalInstruction, uint64(inst))
			return
		}
		v := h.load(addr, width, width == 4)
		if h.trapRaised {
			return
		}
		h.X[rd] = v
		h.Reservation, h.HasReservation = addr, true
		return
	case 0x03: // SC
		if h.HasReservation && h.Reservation == addr {
			h.store(addr, width, h.X[rs2])
			if h.trapRaised {
				return
			}
			h.X[rd] = 0
		} else {
			h.X[rd] = 1
		}
		h.HasReservation = false
		return
	}
	old := h.load(addr, width, false)
	if h.trapRaised {
		return
	}
	src := h.X[rs2]
	maskOld := old
	if width == 4 {
		maskOld = uint64(uint32(old))
		src = uint64(uint32(src))
	}
	var nv uint64
	switch aqrlFunct5 {
	case 0x01:
		nv = src // AMOSWAP
	case 0x00:
		nv = maskOld + src // AMOADD
	case 0x04:
		nv = maskOld ^ src // AMOXOR
	case 0x0c:
		nv = maskOld & src // AMOAND
	case 0x08:
		nv = maskOld | src // AMOOR
	case 0x10: // AMOMIN
		if width == 4 {
			if int32(maskOld) < int32(src) {
				nv = maskOld
			} else {
				nv = src
			}
		} else if int64(maskOld) < int64(src) {
			nv = maskOld
		} else {
			nv = src
		}
	case 0x14: // AMOMAX
		if width == 4 {
			if int32(maskOld) > int32(src) {
				nv = maskOld
			} else {
				nv = src
			}
		} else if int64(maskOld) > int64(src) {
			nv = maskOld
		} else {
			nv = src
		}
	case 0x18:
		if maskOld < src {
			nv = maskOld
		} else {
			nv = src
		} // AMOMINU
	case 0x1c:
		if maskOld > src {
			nv = maskOld
		} else {
			nv = src
		} // AMOMAXU
	default:
		h.raiseException(ExcIllegalInstruction, uint64(inst))
		return
	}
	h.store(addr, width, nv)
	if h.trapRaised {
		return
	}
	if width == 4 {
		h.X[rd] = sext32(uint32(old))
	} else {
		h.X[rd] = old
	}
	if h.HasReservation && h.Reservation == addr {
		h.HasReservation = false
	}
}
