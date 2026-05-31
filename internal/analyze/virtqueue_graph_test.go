package analyze

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestVirtqueueChainsDOTEmpty(t *testing.T) {
	b := mem.NewBus(0x80000000, 4096)
	dot := VirtqueueChainsDOT(b, nil, 8)
	if !strings.Contains(dot, "digraph virtqueue_chains") || !strings.Contains(dot, "empty") {
		t.Fatalf("unexpected DOT: %s", dot)
	}
}

func TestVirtqueueChainsDOT(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x10000)
	desc := uint64(0x80001000)
	avail := uint64(0x80002000)
	used := uint64(0x80003000)
	buf := uint64(0x80004000)
	payload := []byte{1, 2, 3, 4}
	if err := b.Load(buf, payload); err != nil {
		t.Fatal(err)
	}
	var d [16]byte
	binary.LittleEndian.PutUint64(d[0:8], buf)
	binary.LittleEndian.PutUint32(d[8:12], uint32(len(payload)))
	if err := b.Load(desc, d[:]); err != nil {
		t.Fatal(err)
	}
	var a [8]byte
	binary.LittleEndian.PutUint16(a[2:4], 1)
	binary.LittleEndian.PutUint16(a[4:6], 0)
	if err := b.Load(avail, a[:]); err != nil {
		t.Fatal(err)
	}
	var u [4]byte
	if err := b.Load(used, u[:]); err != nil {
		t.Fatal(err)
	}
	events := []mem.AccessEvent{
		{Seq: 1, Name: "virtio-blk", Reg: "QueueSel", Write: true, Value: 0},
		{Seq: 2, Name: "virtio-blk", Reg: "QueueNum", Write: true, Value: 8},
		{Seq: 3, Name: "virtio-blk", Reg: "QueueDescLow", Write: true, Value: desc},
		{Seq: 4, Name: "virtio-blk", Reg: "QueueDriverLow", Write: true, Value: avail},
		{Seq: 5, Name: "virtio-blk", Reg: "QueueDeviceLow", Write: true, Value: used},
		{Seq: 6, Name: "virtio-blk", Reg: "QueueReady", Write: true, Value: 1},
		{Seq: 7, Name: "virtio-blk", Reg: "QueueNotify", Write: true, Value: 0},
	}
	dot := VirtqueueChainsDOT(b, events, 4)
	if !strings.Contains(dot, "virtio-blk q=0") || !strings.Contains(dot, "addr=0x80004000") || !strings.Contains(dot, "01 02 03 04") {
		t.Fatalf("unexpected DOT: %s", dot)
	}
}
