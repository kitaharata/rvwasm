package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestScanMemoryObjects(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x20000)
	copy(b.DRAM[0x1000:], []byte{0x7f, 'E', 'L', 'F', 2, 1})
	copy(b.DRAM[0x2000:], []byte{0xd0, 0x0d, 0xfe, 0xed, 0, 0, 0, 0x40})
	copy(b.DRAM[0x3000:], []byte("Linux version 6.x test\x00"))
	text := ScanMemoryObjectsString(b, 8)
	for _, want := range []string{"elf", "fdt", "linux-version-string"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

func TestClassifyInitcalls(t *testing.T) {
	log := "calling  foo_init+0x0/0x10 @ 1\ninitcall foo_init returned 0 after 12 usecs\nvirtio_blk virtio0: [vda] 2048 512-byte sectors\nprobe failed for demo\n"
	events := ClassifyInitcalls(log, 16)
	if len(events) < 4 {
		t.Fatalf("events=%#v", events)
	}
	counts := InitcallCategoryCounts(log)
	if counts["initcall-return"] == 0 || counts["virtio"] == 0 || counts["driver-fail"] == 0 {
		t.Fatalf("counts=%#v", counts)
	}
}
