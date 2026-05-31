package analyze

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestVirtqueueChains(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x10000)
	desc := uint64(0x80001000)
	avail := uint64(0x80002000)
	used := uint64(0x80003000)
	buf := uint64(0x80004000)
	// desc[0] -> desc[1]
	binary.LittleEndian.PutUint64(b.DRAM[0x1000:], buf)
	binary.LittleEndian.PutUint32(b.DRAM[0x1008:], 4)
	binary.LittleEndian.PutUint16(b.DRAM[0x100c:], vqDescFNext)
	binary.LittleEndian.PutUint16(b.DRAM[0x100e:], 1)
	binary.LittleEndian.PutUint64(b.DRAM[0x1010:], buf+0x10)
	binary.LittleEndian.PutUint32(b.DRAM[0x1018:], 1)
	binary.LittleEndian.PutUint16(b.DRAM[0x101c:], vqDescFWrite)
	copy(b.DRAM[0x4000:], []byte{1, 2, 3, 4})
	binary.LittleEndian.PutUint16(b.DRAM[0x2002:], 1) // avail idx
	binary.LittleEndian.PutUint16(b.DRAM[0x2004:], 0) // ring[0]
	events := []mem.AccessEvent{
		{Name: "virtio-blk", Write: true, Reg: "QueueSel", Value: 0, Seq: 1},
		{Name: "virtio-blk", Write: true, Reg: "QueueNum", Value: 8, Seq: 2},
		{Name: "virtio-blk", Write: true, Reg: "QueueDescLow", Value: desc, Seq: 3},
		{Name: "virtio-blk", Write: true, Reg: "QueueDriverLow", Value: avail, Seq: 4},
		{Name: "virtio-blk", Write: true, Reg: "QueueDeviceLow", Value: used, Seq: 5},
		{Name: "virtio-blk", Write: true, Reg: "QueueReady", Value: 1, Seq: 6},
		{Name: "virtio-blk", Write: true, Reg: "QueueNotify", Value: 0, Seq: 7},
	}
	chains := VirtqueueChains(b, events, 4)
	if len(chains) != 1 {
		t.Fatalf("chains=%d", len(chains))
	}
	if got := len(chains[0].Descriptors); got != 2 {
		t.Fatalf("descriptors=%d", got)
	}
	if !strings.Contains(VirtqueueChainsString(b, events, 4), "data=01 02 03 04") {
		t.Fatal("missing descriptor preview")
	}
}
