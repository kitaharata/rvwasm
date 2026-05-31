package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestScanMemoryObjectsMoreSignatures(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x4000)
	_ = b.Load(0x80000100, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00})
	_ = b.Load(0x80000200, []byte("BusyBox v1.36.0"))
	_ = b.Load(0x80000300, []byte("root=/dev/vda rw"))
	text := ScanMemoryObjectsString(b, 8)
	for _, want := range []string{"xz", "busybox-string", "root-arg-string"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
	summary := MemoryObjectSummaryString(b, 8)
	if !strings.Contains(summary, "busybox-string") {
		t.Fatalf("bad summary: %s", summary)
	}
}
