package dev

import (
	"bytes"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func enableRNG(t *testing.T, rng *VirtioRNG) {
	t.Helper()
	_ = rng.Write(VirtioRNGBase+0x024, 4, 0)
	_ = rng.Write(VirtioRNGBase+0x020, 4, 1<<virtioRingFIndirectDescBit)
	_ = rng.Write(VirtioRNGBase+0x024, 4, virtioFVersion1Bit/32)
	_ = rng.Write(VirtioRNGBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = rng.Write(VirtioRNGBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
}

func setupRNGQueue(t *testing.T, rng *VirtioRNG, desc, avail, used uint64) {
	t.Helper()
	_ = rng.Write(VirtioRNGBase+0x030, 4, 0)
	_ = rng.Write(VirtioRNGBase+0x038, 4, virtioRNGQueueSize)
	_ = rng.Write(VirtioRNGBase+0x080, 4, uint64(uint32(desc)))
	_ = rng.Write(VirtioRNGBase+0x084, 4, desc>>32)
	_ = rng.Write(VirtioRNGBase+0x090, 4, uint64(uint32(avail)))
	_ = rng.Write(VirtioRNGBase+0x094, 4, avail>>32)
	_ = rng.Write(VirtioRNGBase+0x0a0, 4, uint64(uint32(used)))
	_ = rng.Write(VirtioRNGBase+0x0a4, 4, used>>32)
	_ = rng.Write(VirtioRNGBase+0x044, 4, 1)
}

func TestVirtioRNGFillsGuestBuffer(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const data = dram + 0x4000

	bus := mem.NewBus(dram, 0x10000)
	var irq bool
	rng := NewVirtioRNG(bus, func(set bool) { irq = set })
	bus.AddDevice(VirtioRNGBase, 0x200, rng)
	rng.SetSeed(0x1234)
	enableRNG(t, rng)
	setupRNGQueue(t, rng, desc, avail, used)

	writeDesc(t, bus, desc, 0, data, 32, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	_ = rng.Write(VirtioRNGBase+0x050, 4, 0)

	buf := make([]byte, 32)
	for i := range buf {
		x, err := bus.Read(data+uint64(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		buf[i] = byte(x)
	}
	if bytes.Equal(buf, make([]byte, 32)) {
		t.Fatalf("entropy buffer stayed zero")
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
	usedLen, _ := bus.Read(used+4+4, 4)
	if usedLen != 32 {
		t.Fatalf("used len=%d", usedLen)
	}
	if !irq {
		t.Fatalf("rng irq not raised")
	}
}

func TestVirtioRNGIndirectDescriptor(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const indirect = dram + 0x3800
	const data = dram + 0x4000

	bus := mem.NewBus(dram, 0x10000)
	rng := NewVirtioRNG(bus, nil)
	bus.AddDevice(VirtioRNGBase, 0x200, rng)
	enableRNG(t, rng)
	setupRNGQueue(t, rng, desc, avail, used)

	writeDesc(t, bus, desc, 0, indirect, 16, virtqDescFIndirect, 0)
	writeDesc(t, bus, indirect, 0, data, 16, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	_ = rng.Write(VirtioRNGBase+0x050, 4, 0)

	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d err=%s", idx, rng.LastError())
	}
	usedLen, _ := bus.Read(used+4+4, 4)
	if usedLen != 16 {
		t.Fatalf("used len=%d", usedLen)
	}
}
