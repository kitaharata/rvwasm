package analyze

import (
	"strings"
	"testing"
)

func TestInitcallTimelineString(t *testing.T) {
	log := "calling start_kernel+0x0/0x100\ninitcall virtio_mmio_init returned 0\nvirtio-mmio 10001000.virtio: registered\n"
	got := InitcallTimelineString(log, 16)
	for _, want := range []string{"initcall-start", "initcall-return", "virtio_mmio_init"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
}
