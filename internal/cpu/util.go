package cpu

func bit(x uint64, n uint) uint64       { return (x >> n) & 1 }
func bits(x uint64, hi, lo uint) uint64 { return (x >> lo) & ((uint64(1) << (hi - lo + 1)) - 1) }
func sext(v uint64, width uint) uint64  { shift := 64 - width; return uint64(int64(v<<shift) >> shift) }
func sext32(v uint32) uint64            { return uint64(int64(int32(v))) }
func u32(v uint64) uint64               { return uint64(uint32(v)) }
func s64(v uint64) int64                { return int64(v) }

func immI(inst uint32) uint64 { return sext(uint64(inst>>20), 12) }
func immS(inst uint32) uint64 { return sext(uint64((inst>>25)<<5|((inst>>7)&0x1f)), 12) }
func immB(inst uint32) uint64 {
	v := ((inst >> 31) << 12) | (((inst >> 7) & 1) << 11) | (((inst >> 25) & 0x3f) << 5) | (((inst >> 8) & 0xf) << 1)
	return sext(uint64(v), 13)
}
func immU(inst uint32) uint64 { return sext(uint64(inst&0xfffff000), 32) }
func immJ(inst uint32) uint64 {
	v := ((inst >> 31) << 20) | (((inst >> 12) & 0xff) << 12) | (((inst >> 20) & 1) << 11) | (((inst >> 21) & 0x3ff) << 1)
	return sext(uint64(v), 21)
}

func cReg(x uint16) uint64  { return uint64(8 + ((x >> 2) & 7)) }
func cReg2(x uint16) uint64 { return uint64(8 + ((x >> 7) & 7)) }
