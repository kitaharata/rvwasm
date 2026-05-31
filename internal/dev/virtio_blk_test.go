package dev

import (
	"bytes"
	"github.com/kitaharata/rvwasm/internal/mem"
	"testing"
)

func writeDesc(t *testing.T, bus *mem.Bus, base uint64, id uint16, addr uint64, ln uint32, flags uint16, next uint16) {
	t.Helper()
	off := base + uint64(id)*16
	if err := bus.Write(off, 8, addr); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(off+8, 4, uint64(ln)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(off+12, 2, uint64(flags)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(off+14, 2, uint64(next)); err != nil {
		t.Fatal(err)
	}
}

func TestVirtioBlockReadRequest(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const req = dram + 0x4000
	const data = dram + 0x5000
	const status = dram + 0x6000

	bus := mem.NewBus(dram, 0x10000)
	disk := make([]byte, 512)
	copy(disk, []byte("hello virtio"))
	var irq bool
	blk := NewVirtioBlock(bus, disk, func(set bool) { irq = set })
	bus.AddDevice(VirtioBlockBase, 0x200, blk)

	// virtio_blk_outhdr: type=IN, reserved=0, sector=0
	if err := bus.Write(req, 4, uint64(virtioBlkTIn)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(req+8, 8, 0); err != nil {
		t.Fatal(err)
	}
	writeDesc(t, bus, desc, 0, req, 16, virtqDescFNext, 1)
	writeDesc(t, bus, desc, 1, data, 512, virtqDescFNext|virtqDescFWrite, 2)
	writeDesc(t, bus, desc, 2, status, 1, virtqDescFWrite, 0)
	if err := bus.Write(avail+2, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(avail+4, 2, 0); err != nil {
		t.Fatal(err)
	}

	_ = blk.Write(VirtioBlockBase+0x024, 4, virtioFVersion1Bit/32) // DriverFeaturesSel
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = blk.Write(VirtioBlockBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
	_ = blk.Write(VirtioBlockBase+0x030, 4, 0) // QueueSel
	_ = blk.Write(VirtioBlockBase+0x038, 4, virtioQueueSize)
	_ = blk.Write(VirtioBlockBase+0x080, 4, uint64(uint32(desc)))
	_ = blk.Write(VirtioBlockBase+0x084, 4, desc>>32)
	_ = blk.Write(VirtioBlockBase+0x090, 4, uint64(uint32(avail)))
	_ = blk.Write(VirtioBlockBase+0x094, 4, avail>>32)
	_ = blk.Write(VirtioBlockBase+0x0a0, 4, uint64(uint32(used)))
	_ = blk.Write(VirtioBlockBase+0x0a4, 4, used>>32)
	_ = blk.Write(VirtioBlockBase+0x044, 4, 1) // QueueReady
	_ = blk.Write(VirtioBlockBase+0x050, 4, 0) // QueueNotify

	buf := make([]byte, 12)
	for i := range buf {
		v, err := bus.Read(data+uint64(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		buf[i] = byte(v)
	}
	if !bytes.Equal(buf, []byte("hello virtio")) {
		t.Fatalf("data=%q", string(buf))
	}
	st, _ := bus.Read(status, 1)
	if st != 0 {
		t.Fatalf("status=%d", st)
	}
	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
	if !irq {
		t.Fatalf("irq not raised")
	}
	_ = blk.Write(VirtioBlockBase+0x064, 4, 1)
	if irq {
		t.Fatalf("irq still raised after ack")
	}
}

func TestVirtioMMIOIdentityRegisters(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	blk := NewVirtioBlock(bus, nil, nil)
	for _, tc := range []struct {
		off  uint64
		want uint64
	}{
		{0x000, uint64(virtioMMIOMagic)},
		{0x004, uint64(virtioVersionModern)},
		{0x008, uint64(virtioDeviceBlock)},
		{0x00c, uint64(virtioVendorRVWASM)},
	} {
		got, err := blk.Read(VirtioBlockBase+tc.off, 4)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("reg %#x = %#x, want %#x", tc.off, got, tc.want)
		}
	}
}

func TestVirtioFeaturesOKRequiresVersion1(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	blk := NewVirtioBlock(bus, nil, nil)
	_ = blk.Write(VirtioBlockBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures))
	got, _ := blk.Read(VirtioBlockBase+0x070, 4)
	if got&uint64(virtioStatusFeatures) != 0 {
		t.Fatalf("FEATURES_OK accepted without VIRTIO_F_VERSION_1: status=%#x", got)
	}

	_ = blk.Write(VirtioBlockBase+0x024, 4, virtioFVersion1Bit/32) // DriverFeaturesSel
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = blk.Write(VirtioBlockBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures))
	got, _ = blk.Read(VirtioBlockBase+0x070, 4)
	if got&uint64(virtioStatusFeatures) == 0 {
		t.Fatalf("FEATURES_OK rejected after VERSION_1 negotiation: status=%#x", got)
	}
}

func TestVirtioBlockIndirectDescriptorRead(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const indirect = dram + 0x3800
	const req = dram + 0x4000
	const data = dram + 0x5000
	const status = dram + 0x6000

	bus := mem.NewBus(dram, 0x10000)
	disk := make([]byte, 512)
	copy(disk, []byte("indirect-ok"))
	blk := NewVirtioBlock(bus, disk, nil)
	bus.AddDevice(VirtioBlockBase, 0x200, blk)

	if err := bus.Write(req, 4, uint64(virtioBlkTIn)); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(req+8, 8, 0); err != nil {
		t.Fatal(err)
	}
	// Main descriptor points at an indirect table. The indirect table contains
	// the real request header, data buffer and status byte descriptors.
	writeDesc(t, bus, desc, 0, indirect, 3*16, virtqDescFIndirect, 0)
	writeDesc(t, bus, indirect, 0, req, 16, virtqDescFNext, 1)
	writeDesc(t, bus, indirect, 1, data, 512, virtqDescFNext|virtqDescFWrite, 2)
	writeDesc(t, bus, indirect, 2, status, 1, virtqDescFWrite, 0)
	if err := bus.Write(avail+2, 2, 1); err != nil {
		t.Fatal(err)
	}
	if err := bus.Write(avail+4, 2, 0); err != nil {
		t.Fatal(err)
	}

	_ = blk.Write(VirtioBlockBase+0x024, 4, 0) // DriverFeaturesSel low
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<virtioRingFIndirectDescBit)
	_ = blk.Write(VirtioBlockBase+0x024, 4, virtioFVersion1Bit/32)
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = blk.Write(VirtioBlockBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
	_ = blk.Write(VirtioBlockBase+0x030, 4, 0)
	_ = blk.Write(VirtioBlockBase+0x038, 4, virtioQueueSize)
	_ = blk.Write(VirtioBlockBase+0x080, 4, uint64(uint32(desc)))
	_ = blk.Write(VirtioBlockBase+0x084, 4, desc>>32)
	_ = blk.Write(VirtioBlockBase+0x090, 4, uint64(uint32(avail)))
	_ = blk.Write(VirtioBlockBase+0x094, 4, avail>>32)
	_ = blk.Write(VirtioBlockBase+0x0a0, 4, uint64(uint32(used)))
	_ = blk.Write(VirtioBlockBase+0x0a4, 4, used>>32)
	_ = blk.Write(VirtioBlockBase+0x044, 4, 1)
	_ = blk.Write(VirtioBlockBase+0x050, 4, 0)

	buf := make([]byte, len("indirect-ok"))
	for i := range buf {
		v, err := bus.Read(data+uint64(i), 1)
		if err != nil {
			t.Fatal(err)
		}
		buf[i] = byte(v)
	}
	if !bytes.Equal(buf, []byte("indirect-ok")) {
		t.Fatalf("data=%q", string(buf))
	}
	st, _ := bus.Read(status, 1)
	if st != 0 {
		t.Fatalf("status=%d err=%s", st, blk.LastError())
	}
}

func TestVirtioBlockEventIdxSuppressesInterrupt(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const req = dram + 0x4000
	const data = dram + 0x5000
	const status = dram + 0x6000

	bus := mem.NewBus(dram, 0x10000)
	disk := make([]byte, 512)
	copy(disk, []byte("eventidx"))
	var irq bool
	blk := NewVirtioBlock(bus, disk, func(set bool) { irq = set })
	bus.AddDevice(VirtioBlockBase, 0x200, blk)

	_ = bus.Write(req, 4, uint64(virtioBlkTIn))
	_ = bus.Write(req+8, 8, 0)
	writeDesc(t, bus, desc, 0, req, 16, virtqDescFNext, 1)
	writeDesc(t, bus, desc, 1, data, 512, virtqDescFNext|virtqDescFWrite, 2)
	writeDesc(t, bus, desc, 2, status, 1, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	// Used event lives after the used ring. 7 is not crossed by old=0,new=1,
	// so the device must not raise an interrupt when EVENT_IDX is negotiated.
	_ = bus.Write(used+4+virtioQueueSize*8, 2, 7)

	_ = blk.Write(VirtioBlockBase+0x024, 4, 0)
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<virtioRingFEventIdxBit)
	_ = blk.Write(VirtioBlockBase+0x024, 4, virtioFVersion1Bit/32)
	_ = blk.Write(VirtioBlockBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = blk.Write(VirtioBlockBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
	_ = blk.Write(VirtioBlockBase+0x030, 4, 0)
	_ = blk.Write(VirtioBlockBase+0x038, 4, virtioQueueSize)
	_ = blk.Write(VirtioBlockBase+0x080, 4, uint64(uint32(desc)))
	_ = blk.Write(VirtioBlockBase+0x084, 4, desc>>32)
	_ = blk.Write(VirtioBlockBase+0x090, 4, uint64(uint32(avail)))
	_ = blk.Write(VirtioBlockBase+0x094, 4, avail>>32)
	_ = blk.Write(VirtioBlockBase+0x0a0, 4, uint64(uint32(used)))
	_ = blk.Write(VirtioBlockBase+0x0a4, 4, used>>32)
	_ = blk.Write(VirtioBlockBase+0x044, 4, 1)
	_ = blk.Write(VirtioBlockBase+0x050, 4, 0)

	idx, _ := bus.Read(used+2, 2)
	if idx != 1 {
		t.Fatalf("used idx=%d", idx)
	}
	if irq {
		t.Fatalf("EVENT_IDX should have suppressed interrupt")
	}
}
