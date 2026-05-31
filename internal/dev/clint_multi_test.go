package dev

import "testing"

func TestCLINTMultiHartMSIPAndTimer(t *testing.T) {
	pending := map[int]uint64{}
	c := NewCLINTMulti(2, func(hart int, mask uint64, set bool) {
		if set {
			pending[hart] |= mask
		} else {
			pending[hart] &^= mask
		}
	})
	_ = c.Write(CLINTBase+4, 4, 1)
	if pending[1]&MIP_MSIP == 0 || pending[0]&MIP_MSIP != 0 {
		t.Fatalf("bad msip routing: %#v", pending)
	}
	_ = c.Write(CLINTBase+0x4000+8, 8, 3)
	c.Tick(3)
	if pending[1]&MIP_MTIP == 0 || pending[0]&MIP_MTIP != 0 {
		t.Fatalf("bad mtip routing: %#v", pending)
	}
}
