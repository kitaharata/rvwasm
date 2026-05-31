package dev

import (
	"encoding/binary"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func enableInput(t *testing.T, vi *VirtioInput) {
	t.Helper()
	_ = vi.Write(VirtioInputBase+0x024, 4, 0)
	_ = vi.Write(VirtioInputBase+0x020, 4, 1<<virtioRingFIndirectDescBit)
	_ = vi.Write(VirtioInputBase+0x024, 4, virtioFVersion1Bit/32)
	_ = vi.Write(VirtioInputBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = vi.Write(VirtioInputBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
}

func setupInputQueue(t *testing.T, vi *VirtioInput, qsel uint32, desc, avail, used uint64) {
	t.Helper()
	_ = vi.Write(VirtioInputBase+0x030, 4, uint64(qsel))
	_ = vi.Write(VirtioInputBase+0x038, 4, virtioInputQueueSize)
	_ = vi.Write(VirtioInputBase+0x080, 4, uint64(uint32(desc)))
	_ = vi.Write(VirtioInputBase+0x084, 4, desc>>32)
	_ = vi.Write(VirtioInputBase+0x090, 4, uint64(uint32(avail)))
	_ = vi.Write(VirtioInputBase+0x094, 4, avail>>32)
	_ = vi.Write(VirtioInputBase+0x0a0, 4, uint64(uint32(used)))
	_ = vi.Write(VirtioInputBase+0x0a4, 4, used>>32)
	_ = vi.Write(VirtioInputBase+0x044, 4, 1)
}

func TestVirtioInputInjectKeyEvent(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const data = dram + 0x4000

	bus := mem.NewBus(dram, 0x10000)
	var irq bool
	vi := NewVirtioInput(bus, func(set bool) { irq = set })
	bus.AddDevice(VirtioInputBase, 0x200, vi)
	enableInput(t, vi)
	setupInputQueue(t, vi, virtioInputQueueEvent, desc, avail, used)

	writeDesc(t, bus, desc, 0, data, 16, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	vi.InjectKey(28, true)

	buf := make([]byte, 16)
	for i := range buf {
		x, err := bus.Read(data+uint64(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		buf[i] = byte(x)
	}
	if typ := binary.LittleEndian.Uint16(buf[0:2]); typ != virtioInputEvKey {
		t.Fatalf("event type=%#x", typ)
	}
	if code := binary.LittleEndian.Uint16(buf[2:4]); code != 28 {
		t.Fatalf("event code=%#x", code)
	}
	if val := binary.LittleEndian.Uint32(buf[4:8]); val != 1 {
		t.Fatalf("event value=%#x", val)
	}
	if typ := binary.LittleEndian.Uint16(buf[8:10]); typ != virtioInputEvSyn {
		t.Fatalf("syn type=%#x", typ)
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d err=%s", idx, vi.LastError())
	}
	usedLen, _ := bus.Read(used+4+4, 4)
	if usedLen != 16 {
		t.Fatalf("used len=%d", usedLen)
	}
	if !irq {
		t.Fatalf("input irq not raised")
	}
}

func TestVirtioInputConfigName(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	vi := NewVirtioInput(bus, nil)
	_ = vi.Write(VirtioInputBase+0x100, 1, uint64(virtioInputCfgIDName))
	name0, err := vi.Read(VirtioInputBase+0x108, 1)
	if err != nil {
		t.Fatal(err)
	}
	if byte(name0) != 'r' {
		t.Fatalf("name[0]=%#x", name0)
	}
}
