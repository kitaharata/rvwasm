package cpu

import mathbits "math/bits"

func (h *Hart) execM(rd uint64, a uint64, b uint64, funct3 uint32) {
	switch funct3 {
	case 0: // MUL
		h.X[rd] = a * b
	case 1: // MULH
		_, hi := mathbits.Mul64(uint64(s64(a)), uint64(s64(b)))
		// mathbits.Mul64 is unsigned; correction for signed high product.
		hi = mulhSigned(a, b)
		h.X[rd] = hi
	case 2: // MULHSU
		h.X[rd] = mulhSignedUnsigned(a, b)
	case 3: // MULHU
		hi, _ := mathbits.Mul64(a, b)
		h.X[rd] = hi
	case 4: // DIV
		if b == 0 {
			h.X[rd] = ^uint64(0)
		} else if a == 0x8000000000000000 && b == ^uint64(0) {
			h.X[rd] = a
		} else {
			h.X[rd] = uint64(s64(a) / s64(b))
		}
	case 5: // DIVU
		if b == 0 {
			h.X[rd] = ^uint64(0)
		} else {
			h.X[rd] = a / b
		}
	case 6: // REM
		if b == 0 {
			h.X[rd] = a
		} else if a == 0x8000000000000000 && b == ^uint64(0) {
			h.X[rd] = 0
		} else {
			h.X[rd] = uint64(s64(a) % s64(b))
		}
	case 7: // REMU
		if b == 0 {
			h.X[rd] = a
		} else {
			h.X[rd] = a % b
		}
	}
}

func (h *Hart) execM32(rd uint64, a uint64, b uint64, funct3 uint32) {
	aa, bb := uint32(a), uint32(b)
	switch funct3 {
	case 0:
		h.X[rd] = sext32(aa * bb)
	case 4:
		if bb == 0 {
			h.X[rd] = ^uint64(0)
		} else if aa == 0x80000000 && bb == 0xffffffff {
			h.X[rd] = sext32(aa)
		} else {
			h.X[rd] = sext32(uint32(int32(aa) / int32(bb)))
		}
	case 5:
		if bb == 0 {
			h.X[rd] = ^uint64(0)
		} else {
			h.X[rd] = sext32(aa / bb)
		}
	case 6:
		if bb == 0 {
			h.X[rd] = sext32(aa)
		} else if aa == 0x80000000 && bb == 0xffffffff {
			h.X[rd] = 0
		} else {
			h.X[rd] = sext32(uint32(int32(aa) % int32(bb)))
		}
	case 7:
		if bb == 0 {
			h.X[rd] = sext32(aa)
		} else {
			h.X[rd] = sext32(aa % bb)
		}
	default:
		h.raiseException(ExcIllegalInstruction, 0)
	}
}

func mulhSigned(a, b uint64) uint64 {
	hi, _ := mathbits.Mul64(a, b)
	if int64(a) < 0 {
		hi -= b
	}
	if int64(b) < 0 {
		hi -= a
	}
	return hi
}

func mulhSignedUnsigned(a, b uint64) uint64 {
	hi, _ := mathbits.Mul64(a, b)
	if int64(a) < 0 {
		hi -= b
	}
	return hi
}
