package dev

import "encoding/binary"

const CLINTBase uint64 = 0x02000000

const (
	MIP_MSIP uint64 = 1 << 3
	MIP_MTIP uint64 = 1 << 7
)

type CLINT struct {
	MSIP       []uint32
	MTimeCmp   []uint64
	MTime      uint64
	SetPending func(hart int, mask uint64, set bool)
}

func NewCLINT(setter func(mask uint64, set bool)) *CLINT {
	return NewCLINTMulti(1, func(_ int, mask uint64, pending bool) { setter(mask, pending) })
}

func NewCLINTMulti(harts int, set func(hart int, mask uint64, set bool)) *CLINT {
	if harts <= 0 {
		harts = 1
	}
	c := &CLINT{MSIP: make([]uint32, harts), MTimeCmp: make([]uint64, harts), SetPending: set}
	for i := range c.MTimeCmp {
		c.MTimeCmp[i] = ^uint64(0)
	}
	return c
}

func (c *CLINT) HartCount() int { return len(c.MSIP) }

func (c *CLINT) update() {
	if c.SetPending == nil {
		return
	}
	for hart := range c.MSIP {
		c.SetPending(hart, MIP_MSIP, c.MSIP[hart]&1 != 0)
		c.SetPending(hart, MIP_MTIP, c.MTime >= c.MTimeCmp[hart])
	}
}

func (c *CLINT) Read(addr uint64, size int) (uint64, error) {
	off := addr - CLINTBase
	if off < 0x4000 {
		hart := int(off / 4)
		if hart >= 0 && hart < len(c.MSIP) {
			var tmp [4]byte
			binary.LittleEndian.PutUint32(tmp[:], c.MSIP[hart])
			return readLE(tmp[:], int(off%4), size), nil
		}
		return 0, nil
	}
	if off >= 0x4000 && off < 0x4000+uint64(len(c.MTimeCmp))*8 {
		hart := int((off - 0x4000) / 8)
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], c.MTimeCmp[hart])
		return readLE(tmp[:], int((off-0x4000)%8), size), nil
	}
	if off >= 0xbff8 && off < 0xc000 {
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], c.MTime)
		return readLE(tmp[:], int(off-0xbff8), size), nil
	}
	return 0, nil
}

func (c *CLINT) Write(addr uint64, size int, val uint64) error {
	off := addr - CLINTBase
	switch {
	case off < 0x4000:
		hart := int(off / 4)
		if hart >= 0 && hart < len(c.MSIP) {
			var tmp [4]byte
			binary.LittleEndian.PutUint32(tmp[:], c.MSIP[hart])
			writeLE(tmp[:], int(off%4), size, val)
			c.MSIP[hart] = binary.LittleEndian.Uint32(tmp[:])
		}
	case off >= 0x4000 && off < 0x4000+uint64(len(c.MTimeCmp))*8:
		hart := int((off - 0x4000) / 8)
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], c.MTimeCmp[hart])
		writeLE(tmp[:], int((off-0x4000)%8), size, val)
		c.MTimeCmp[hart] = binary.LittleEndian.Uint64(tmp[:])
	case off >= 0xbff8 && off < 0xc000:
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], c.MTime)
		writeLE(tmp[:], int(off-0xbff8), size, val)
		c.MTime = binary.LittleEndian.Uint64(tmp[:])
	}
	c.update()
	return nil
}

func (c *CLINT) Tick(cycles uint64) {
	c.MTime += cycles
	// During early OpenSBI boot mtimecmp is normally all-ones, so repeatedly
	// calling SetPending on every instruction is pure overhead. Writes to MSIP or
	// MTIMECMP still call update() immediately; Tick only needs to update when a
	// timer compare has actually become pending.
	for _, cmp := range c.MTimeCmp {
		if c.MTime >= cmp {
			c.update()
			return
		}
	}
}

func readLE(buf []byte, off, size int) uint64 {
	if off < 0 || off+size > len(buf) {
		return 0
	}
	switch size {
	case 1:
		return uint64(buf[off])
	case 2:
		return uint64(binary.LittleEndian.Uint16(buf[off:]))
	case 4:
		return uint64(binary.LittleEndian.Uint32(buf[off:]))
	case 8:
		return binary.LittleEndian.Uint64(buf[off:])
	default:
		return 0
	}
}

func writeLE(buf []byte, off, size int, val uint64) {
	if off < 0 || off+size > len(buf) {
		return
	}
	switch size {
	case 1:
		buf[off] = byte(val)
	case 2:
		binary.LittleEndian.PutUint16(buf[off:], uint16(val))
	case 4:
		binary.LittleEndian.PutUint32(buf[off:], uint32(val))
	case 8:
		binary.LittleEndian.PutUint64(buf[off:], val)
	}
}

func (c *CLINT) SetTimerCompare(v uint64) { c.SetTimerCompareHart(0, v) }

func (c *CLINT) SetTimerCompareHart(hart int, v uint64) {
	if hart >= 0 && hart < len(c.MTimeCmp) {
		c.MTimeCmp[hart] = v
	}
	c.update()
}

func (c *CLINT) SetSoftwareInterrupt(v bool) { c.SetSoftwareInterruptHart(0, v) }

func (c *CLINT) SetSoftwareInterruptHart(hart int, v bool) {
	if hart >= 0 && hart < len(c.MSIP) {
		if v {
			c.MSIP[hart] = 1
		} else {
			c.MSIP[hart] = 0
		}
	}
	c.update()
}

func (c *CLINT) DebugString() string {
	return "mtime=" + formatHex(c.MTime) + " mtimecmp0=" + formatHex(c.MTimeCmp[0]) + " msip0=" + formatHex(uint64(c.MSIP[0])) + " harts=" + formatHex(uint64(len(c.MSIP)))
}

func formatHex(v uint64) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0x0"
	}
	var buf [18]byte
	i := len(buf)
	for v != 0 {
		i--
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	i--
	buf[i] = 'x'
	i--
	buf[i] = '0'
	return string(buf[i:])
}
