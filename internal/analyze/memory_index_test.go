package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestMemoryIndexGroupsNearbyObjects(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x20000)
	if err := b.Load(0x80001000, []byte("Linux version test\x00root=/dev/ram0\x00")); err != nil {
		t.Fatal(err)
	}
	if err := b.Load(0x80001100, []byte{0x1f, 0x8b, 8, 0}); err != nil {
		t.Fatal(err)
	}
	s := MemoryIndexString(b, 8)
	if !strings.Contains(s, "linux-version-string") || !strings.Contains(s, "root-arg-string") || !strings.Contains(s, "gzip") {
		t.Fatalf("index missing expected signatures:\n%s", s)
	}
}
