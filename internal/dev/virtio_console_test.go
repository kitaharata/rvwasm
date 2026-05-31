package dev

import (
	"bytes"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func setupConsoleQueue(t *testing.T, vc *VirtioConsole, qsel uint32, desc, avail, used uint64) {
	t.Helper()
	_ = vc.Write(VirtioConsoleBase+0x030, 4, uint64(qsel))
	_ = vc.Write(VirtioConsoleBase+0x038, 4, virtioConsoleQueueSize)
	_ = vc.Write(VirtioConsoleBase+0x080, 4, uint64(uint32(desc)))
	_ = vc.Write(VirtioConsoleBase+0x084, 4, desc>>32)
	_ = vc.Write(VirtioConsoleBase+0x090, 4, uint64(uint32(avail)))
	_ = vc.Write(VirtioConsoleBase+0x094, 4, avail>>32)
	_ = vc.Write(VirtioConsoleBase+0x0a0, 4, uint64(uint32(used)))
	_ = vc.Write(VirtioConsoleBase+0x0a4, 4, used>>32)
	_ = vc.Write(VirtioConsoleBase+0x044, 4, 1)
}

func enableConsole(t *testing.T, vc *VirtioConsole) {
	t.Helper()
	_ = vc.Write(VirtioConsoleBase+0x024, 4, 0)
	_ = vc.Write(VirtioConsoleBase+0x020, 4, 1<<virtioConsoleFSizeBit)
	_ = vc.Write(VirtioConsoleBase+0x024, 4, virtioFVersion1Bit/32)
	_ = vc.Write(VirtioConsoleBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = vc.Write(VirtioConsoleBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
}

func TestVirtioConsoleTransmitQueue(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const data = dram + 0x4000

	bus := mem.NewBus(dram, 0x10000)
	var out bytes.Buffer
	var irq bool
	vc := NewVirtioConsole(bus, func(b byte) { out.WriteByte(b) }, func(set bool) { irq = set })
	bus.AddDevice(VirtioConsoleBase, 0x200, vc)
	enableConsole(t, vc)
	setupConsoleQueue(t, vc, virtioConsoleQueueTx, desc, avail, used)

	msg := []byte("hello hvc0\n")
	if err := bus.Load(data, msg); err != nil {
		t.Fatal(err)
	}
	writeDesc(t, bus, desc, 0, data, uint32(len(msg)), 0, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	_ = vc.Write(VirtioConsoleBase+0x050, 4, uint64(virtioConsoleQueueTx))

	if out.String() != string(msg) {
		t.Fatalf("tx=%q", out.String())
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
	if !irq {
		t.Fatalf("tx irq not raised")
	}
}

func TestVirtioConsoleReceiveQueue(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const data = dram + 0x4000

	bus := mem.NewBus(dram, 0x10000)
	var irq bool
	vc := NewVirtioConsole(bus, nil, func(set bool) { irq = set })
	bus.AddDevice(VirtioConsoleBase, 0x200, vc)
	enableConsole(t, vc)
	setupConsoleQueue(t, vc, virtioConsoleQueueRx, desc, avail, used)

	writeDesc(t, bus, desc, 0, data, 16, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	vc.Inject([]byte("abc"))

	buf := make([]byte, 3)
	for i := range buf {
		x, err := bus.Read(data+uint64(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		buf[i] = byte(x)
	}
	if string(buf) != "abc" {
		t.Fatalf("rx=%q", string(buf))
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
	usedLen, _ := bus.Read(used+4+4, 4)
	if usedLen != 3 {
		t.Fatalf("used len=%d", usedLen)
	}
	if !irq {
		t.Fatalf("rx irq not raised")
	}
}
