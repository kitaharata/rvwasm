package analyze

import (
	"encoding/binary"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestVirtqueueAnomaliesDetectIncompleteReadyQueue(t *testing.T) {
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-blk", Reg: "QueueSel", Write: true, Value: 0},
		{Seq: 2, Name: "virtio-blk", Reg: "QueueNum", Write: true, Value: 8},
		{Seq: 3, Name: "virtio-blk", Reg: "QueueReady", Write: true, Value: 1},
	}
	b := mem.NewBus(0x80000000, 0x10000)
	rows := VirtqueueAnomalies(b, events, 8)
	if len(rows) == 0 {
		t.Fatal("expected anomaly")
	}
}

func TestVirtqueueAnomaliesDetectDescriptorOutsideDRAM(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x20000)
	desc := uint64(0x80001000)
	driver := uint64(0x80002000)
	device := uint64(0x80003000)
	// desc[0]: addr outside DRAM, len 16, no next.
	raw := make([]byte, 16)
	binary.LittleEndian.PutUint64(raw[0:8], 0x90000000)
	binary.LittleEndian.PutUint32(raw[8:12], 16)
	if err := b.Load(desc, raw); err != nil {
		t.Fatal(err)
	}
	av := make([]byte, 8)
	binary.LittleEndian.PutUint16(av[2:4], 1) // avail idx
	binary.LittleEndian.PutUint16(av[4:6], 0) // ring[0]
	if err := b.Load(driver, av); err != nil {
		t.Fatal(err)
	}
	if err := b.Load(device, make([]byte, 8)); err != nil {
		t.Fatal(err)
	}
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-blk", Reg: "QueueSel", Write: true, Value: 0},
		{Seq: 2, Name: "virtio-blk", Reg: "QueueNum", Write: true, Value: 8},
		{Seq: 3, Name: "virtio-blk", Reg: "QueueDescLow", Write: true, Value: desc},
		{Seq: 4, Name: "virtio-blk", Reg: "QueueDriverLow", Write: true, Value: driver},
		{Seq: 5, Name: "virtio-blk", Reg: "QueueDeviceLow", Write: true, Value: device},
		{Seq: 6, Name: "virtio-blk", Reg: "QueueReady", Write: true, Value: 1},
		{Seq: 7, Name: "virtio-blk", Reg: "QueueNotify", Write: true, Value: 0},
	}
	rows := VirtqueueAnomalies(b, events, 8)
	found := false
	for _, r := range rows {
		if r.Severity == "error" && r.Queue == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected descriptor outside DRAM anomaly, got %#v", rows)
	}
}
