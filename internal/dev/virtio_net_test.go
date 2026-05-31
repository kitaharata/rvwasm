package dev

import (
	"bytes"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func setupVirtioNetQueue(t *testing.T, bus *mem.Bus, net *VirtioNet, qsel uint32, desc, avail, used uint64) {
	t.Helper()
	_ = net.Write(VirtioNetBase+0x024, 4, virtioFVersion1Bit/32)
	_ = net.Write(VirtioNetBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = net.Write(VirtioNetBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
	_ = net.Write(VirtioNetBase+0x030, 4, uint64(qsel))
	_ = net.Write(VirtioNetBase+0x038, 4, virtioNetQueueSize)
	_ = net.Write(VirtioNetBase+0x080, 4, uint64(uint32(desc)))
	_ = net.Write(VirtioNetBase+0x084, 4, desc>>32)
	_ = net.Write(VirtioNetBase+0x090, 4, uint64(uint32(avail)))
	_ = net.Write(VirtioNetBase+0x094, 4, avail>>32)
	_ = net.Write(VirtioNetBase+0x0a0, 4, uint64(uint32(used)))
	_ = net.Write(VirtioNetBase+0x0a4, 4, used>>32)
	_ = net.Write(VirtioNetBase+0x044, 4, 1)
}

func TestVirtioNetTxCapturesEthernetFrame(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const pkt = dram + 0x4000
	bus := mem.NewBus(dram, 0x10000)
	net := NewVirtioNet(bus, [6]byte{}, nil)
	bus.AddDevice(VirtioNetBase, 0x200, net)
	frame := append(make([]byte, virtioNetHdrLen), []byte{0xde, 0xad, 0xbe, 0xef}...)
	if err := bus.Load(pkt, frame); err != nil {
		t.Fatal(err)
	}
	writeDesc(t, bus, desc, 0, pkt, uint32(len(frame)), 0, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	setupVirtioNetQueue(t, bus, net, virtioNetQueueTx, desc, avail, used)
	_ = net.Write(VirtioNetBase+0x050, 4, uint64(virtioNetQueueTx))
	if len(net.txFrames) != 1 {
		t.Fatalf("tx frames=%d err=%s", len(net.txFrames), net.LastError())
	}
	if !bytes.Equal(net.txFrames[0], []byte{0xde, 0xad, 0xbe, 0xef}) {
		t.Fatalf("frame=%x", net.txFrames[0])
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
}

func TestVirtioNetRxWritesHeaderAndFrame(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const buf = dram + 0x5000
	bus := mem.NewBus(dram, 0x10000)
	net := NewVirtioNet(bus, [6]byte{}, nil)
	bus.AddDevice(VirtioNetBase, 0x200, net)
	writeDesc(t, bus, desc, 0, buf, 64, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	setupVirtioNetQueue(t, bus, net, virtioNetQueueRx, desc, avail, used)
	net.InjectFrame([]byte{1, 2, 3, 4, 5})
	got := make([]byte, virtioNetHdrLen+5)
	for i := range got {
		x, _ := bus.Read(buf+uint64(i), 1)
		got[i] = byte(x)
	}
	want := append(make([]byte, virtioNetHdrLen), []byte{1, 2, 3, 4, 5}...)
	if !bytes.Equal(got, want) {
		t.Fatalf("rx=%x want=%x", got, want)
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
}
