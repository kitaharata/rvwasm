package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestVirtqueueAnomalyHints(t *testing.T) {
	rows := []VirtqueueAnomaly{{Severity: "error", Device: "virtio-blk", Queue: 0, Detail: "ready queue has incomplete addresses desc=0x0 driver=0x0 device=0x0"}}
	s := VirtqueueAnomalyHintsString(rows)
	if !strings.Contains(s, "QueueDescLow") {
		t.Fatalf("hint did not mention register setup: %s", s)
	}
}

func TestQueryDiagnostics(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	idx := BuildMemoryIndex(b, 4, 0)
	hits := QueryDiagnostics("virtio QueueReady", "virtio device found", "pc=80000000 asm=\"addi\"", "", []mem.AccessEvent{{Seq: 1, Name: "virtio-blk", Reg: "QueueReady", Write: true, Addr: 0x10001044, Size: 4, Value: 1}}, nil, idx, 8)
	if len(hits) == 0 || hits[0].Source != "mmio" {
		t.Fatalf("expected mmio hit, got %#v", hits)
	}
}

func TestShareBundleMarkdown(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	bundle := BuildShareBundle("ok", "Linux version test", "pc=80000000 asm=\"ecall\"", "", b, nil, nil, "Linux")
	md := ShareBundleMarkdown(bundle)
	for _, want := range []string{"boot regression report", "Virtqueue anomaly hints", "Diagnostic query hits"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in markdown:\n%s", want, md)
		}
	}
}
