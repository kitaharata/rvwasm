package dev

import (
	"encoding/binary"
	"github.com/kitaharata/rvwasm/internal/mem"
	"strings"
	"testing"
)

func enableGPU(t *testing.T, gpu *VirtioGPU) {
	t.Helper()
	_ = gpu.Write(VirtioGPUBase+0x024, 4, 0)
	_ = gpu.Write(VirtioGPUBase+0x020, 4, 1<<virtioRingFIndirectDescBit)
	_ = gpu.Write(VirtioGPUBase+0x024, 4, virtioFVersion1Bit/32)
	_ = gpu.Write(VirtioGPUBase+0x020, 4, 1<<(virtioFVersion1Bit%32))
	_ = gpu.Write(VirtioGPUBase+0x070, 4, uint64(virtioStatusAck|virtioStatusDriver|virtioStatusFeatures|virtioStatusDriverOK))
}

func setupGPUQueue(t *testing.T, gpu *VirtioGPU, desc, avail, used uint64) {
	t.Helper()
	_ = gpu.Write(VirtioGPUBase+0x030, 4, uint64(virtioGPUQueueControl))
	_ = gpu.Write(VirtioGPUBase+0x038, 4, virtioGPUQueueSize)
	_ = gpu.Write(VirtioGPUBase+0x080, 4, uint64(uint32(desc)))
	_ = gpu.Write(VirtioGPUBase+0x084, 4, desc>>32)
	_ = gpu.Write(VirtioGPUBase+0x090, 4, uint64(uint32(avail)))
	_ = gpu.Write(VirtioGPUBase+0x094, 4, avail>>32)
	_ = gpu.Write(VirtioGPUBase+0x0a0, 4, uint64(uint32(used)))
	_ = gpu.Write(VirtioGPUBase+0x0a4, 4, used>>32)
	_ = gpu.Write(VirtioGPUBase+0x044, 4, 1)
}

func TestVirtioGPUIdentityAndConfig(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	gpu := NewVirtioGPU(bus, nil)
	got, err := gpu.Read(VirtioGPUBase+0x008, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != uint64(virtioDeviceGPU) {
		t.Fatalf("device id=%#x", got)
	}
	nscanouts, _ := gpu.Read(VirtioGPUBase+0x108, 4)
	if nscanouts != 1 {
		t.Fatalf("num_scanouts=%d", nscanouts)
	}
}

func TestVirtioGPUDisplayInfoCommand(t *testing.T) {
	const dram = 0x80000000
	const desc = dram + 0x1000
	const avail = dram + 0x2000
	const used = dram + 0x3000
	const req = dram + 0x4000
	const resp = dram + 0x5000

	bus := mem.NewBus(dram, 0x20000)
	var irq bool
	gpu := NewVirtioGPU(bus, func(set bool) { irq = set })
	bus.AddDevice(VirtioGPUBase, 0x200, gpu)
	enableGPU(t, gpu)
	setupGPUQueue(t, gpu, desc, avail, used)

	_ = bus.Write(req, 4, uint64(virtioGPUCmdGetDisplayInfo))
	writeDesc(t, bus, desc, 0, req, 24, virtqDescFNext, 1)
	writeDesc(t, bus, desc, 1, resp, 24+16*24, virtqDescFWrite, 0)
	_ = bus.Write(avail+2, 2, 1)
	_ = bus.Write(avail+4, 2, 0)
	_ = gpu.Write(VirtioGPUBase+0x050, 4, uint64(virtioGPUQueueControl))

	typ, _ := bus.Read(resp, 4)
	if typ != uint64(virtioGPURespOKDisplayInfo) {
		t.Fatalf("response type=%#x err=%s", typ, gpu.LastError())
	}
	w, _ := bus.Read(resp+24+8, 4)
	h, _ := bus.Read(resp+24+12, 4)
	if w != 1024 || h != 768 {
		t.Fatalf("mode=%dx%d", w, h)
	}
	if !irq {
		t.Fatalf("gpu irq not raised")
	}
	if !strings.Contains(gpu.DebugString(), "commands=1") {
		t.Fatalf("debug did not count command: %s", gpu.DebugString())
	}
}

func TestVirtioGPUResourceCreate2D(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	gpu := NewVirtioGPU(bus, nil)
	cmd := make([]byte, 40)
	binary.LittleEndian.PutUint32(cmd[0:4], virtioGPUCmdResourceCreate2D)
	binary.LittleEndian.PutUint32(cmd[24:28], 7)
	binary.LittleEndian.PutUint32(cmd[32:36], 640)
	binary.LittleEndian.PutUint32(cmd[36:40], 480)
	resp := gpu.gpuResponse(cmd)
	if binary.LittleEndian.Uint32(resp[0:4]) != virtioGPURespOKNoData {
		t.Fatalf("bad response")
	}
	if r := gpu.resources[7]; r.Width != 640 || r.Height != 480 {
		t.Fatalf("resource=%+v", r)
	}
}

func TestVirtioGPUCopiesBackingToFramebuffer(t *testing.T) {
	const dram = 0x80000000
	const backing = dram + 0x2000
	const fb = dram + 0x10000
	bus := mem.NewBus(dram, 0x20000)
	gpu := NewVirtioGPU(bus, nil)
	gpu.SetFramebuffer(fb, 2, 2, 8)

	create := make([]byte, 40)
	binary.LittleEndian.PutUint32(create[0:4], virtioGPUCmdResourceCreate2D)
	binary.LittleEndian.PutUint32(create[24:28], 9)
	binary.LittleEndian.PutUint32(create[32:36], 2)
	binary.LittleEndian.PutUint32(create[36:40], 2)
	gpu.gpuResponse(create)

	attach := make([]byte, 48)
	binary.LittleEndian.PutUint32(attach[0:4], virtioGPUCmdResourceAttachBack)
	binary.LittleEndian.PutUint32(attach[24:28], 9)
	binary.LittleEndian.PutUint32(attach[28:32], 1)
	binary.LittleEndian.PutUint64(attach[32:40], backing)
	binary.LittleEndian.PutUint32(attach[40:44], 16)
	gpu.gpuResponse(attach)

	pixels := []uint32{0xff112233, 0xff445566, 0xff778899, 0xffaabbcc}
	for i, px := range pixels {
		if err := bus.Write(backing+uint64(i*4), 4, uint64(px)); err != nil {
			t.Fatal(err)
		}
	}
	transfer := make([]byte, 56)
	binary.LittleEndian.PutUint32(transfer[0:4], virtioGPUCmdTransferToHost2D)
	binary.LittleEndian.PutUint32(transfer[48:52], 9)
	gpu.gpuResponse(transfer)
	got, err := bus.Read(fb, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != uint64(pixels[0]) {
		t.Fatalf("fb[0]=%#x want %#x debug=%s", got, pixels[0], gpu.DebugString())
	}
	got, err = bus.Read(fb+12, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != uint64(pixels[3]) {
		t.Fatalf("fb[3]=%#x want %#x debug=%s", got, pixels[3], gpu.DebugString())
	}
	if !strings.Contains(gpu.DebugString(), "lastCopy=16") {
		t.Fatalf("copy not reported: %s", gpu.DebugString())
	}
}

func TestVirtioGPUCursorCommandState(t *testing.T) {
	bus := mem.NewBus(0x80000000, 0x10000)
	gpu := NewVirtioGPU(bus, nil)
	cmd := make([]byte, 48)
	binary.LittleEndian.PutUint32(cmd[0:4], virtioGPUCmdUpdateCursor)
	binary.LittleEndian.PutUint32(cmd[24:28], 0)  // scanout
	binary.LittleEndian.PutUint32(cmd[28:32], 11) // x
	binary.LittleEndian.PutUint32(cmd[32:36], 22) // y
	binary.LittleEndian.PutUint32(cmd[36:40], 5)  // resource
	binary.LittleEndian.PutUint32(cmd[40:44], 1)  // hot x
	binary.LittleEndian.PutUint32(cmd[44:48], 2)  // hot y
	resp := gpu.gpuResponseForQueue(cmd, virtioGPUQueueCursor)
	if binary.LittleEndian.Uint32(resp[0:4]) != virtioGPURespOKNoData {
		t.Fatalf("bad response")
	}
	state := gpu.CursorState()
	if state["updates"] != 1 || state["resource_id"] != 5 || state["x"] != 11 || state["y"] != 22 || state["hot_x"] != 1 || state["hot_y"] != 2 {
		t.Fatalf("bad cursor state: %#v debug=%s", state, gpu.DebugString())
	}
	if !strings.Contains(gpu.DebugString(), "cursor=res5") {
		t.Fatalf("cursor not in debug string: %s", gpu.DebugString())
	}
}
