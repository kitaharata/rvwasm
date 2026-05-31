package analyze

import (
	"strings"
	"testing"
)

func testMemoryIndex() MemoryIndex {
	return MemoryIndex{DRAMBase: 0x80000000, DRAMSize: 0x100000, Counts: map[string]int{"elf": 1, "fdt": 1}, Ranges: []MemoryIndexRange{{Start: 0x80000000, End: 0x80002000, Label: "elf=1 fdt=1", Objects: []MemoryObject{{Addr: 0x80000000, Size: 0x1000, Type: "elf", Detail: "ELF header"}, {Addr: 0x80001000, Size: 0x400, Type: "fdt", Detail: "riscv-virtio"}}}}}
}

func TestSearchMemoryIndexAndJumpHints(t *testing.T) {
	idx := testMemoryIndex()
	if hits := SearchMemoryIndex(idx, "fdt", 8); len(hits) == 0 || hits[0].Addr != 0x80000000 {
		t.Fatalf("expected fdt hit/range, got %#v", hits)
	}
	if hits := SearchMemoryIndex(idx, "0x80001010", 8); len(hits) < 2 {
		t.Fatalf("expected address range/object hits, got %#v", hits)
	}
	if hints := MemoryJumpHints(idx, 4); len(hints) == 0 || hints[0].Address != 0x80000000 {
		t.Fatalf("bad hints: %#v", hints)
	}
}

func TestShareBundleHTML(t *testing.T) {
	bundle := ShareBundle{Report: BootRegressionReport{Status: "halted"}, Memory: testMemoryIndex(), JumpHints: MemoryJumpHints(testMemoryIndex(), 4), Anomalies: []VirtqueueAnomaly{{Severity: "error", Device: "virtio", Detail: "QueueReady=1 with QueueNum=0"}}}
	html := ShareBundleHTML(bundle)
	if !strings.Contains(html, "rvwasm share report") || !strings.Contains(html, "application/json") || !strings.Contains(html, "Memory jump hints") {
		t.Fatalf("share html missing sections: %s", html)
	}
}
