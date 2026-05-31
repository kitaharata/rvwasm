package analyze

import (
	"strings"
	"testing"
)

func TestTraceStatsAndCompare(t *testing.T) {
	base := "cycle=1 mode=3 pc=0000000080000000 inst=00000513 asm=\"addi a0,zero,0\" next=0000000080000004\n" +
		"cycle=2 trap interrupt=false cause=2 tval=0x0 target=3 pc=0000000080000004\n"
	st := TraceStatsForText(base)
	if st.Steps != 1 || st.Traps != 1 || st.FirstPC != 0x80000000 || st.LastPC != 0x80000004 {
		t.Fatalf("bad stats: %+v", st)
	}
	other := strings.Replace(base, "pc=0000000080000000", "pc=0000000080000008", 1)
	diffs := CompareTraceText(base, other, 4)
	if len(diffs) == 0 || !strings.Contains(diffs[0].Why, "pc") {
		t.Fatalf("expected pc diff, got %+v", diffs)
	}
}

func TestMemoryObjectDiff(t *testing.T) {
	before := []MemoryObject{{Type: "elf", Addr: 0x80000000, Detail: "old"}}
	after := []MemoryObject{{Type: "elf", Addr: 0x80000000, Detail: "old"}, {Type: "fdt", Addr: 0x87e00000, Size: 0x1000}}
	s := DiffMemoryObjectsString(before, after)
	if !strings.Contains(s, "+") || !strings.Contains(s, "fdt") {
		t.Fatalf("bad diff: %s", s)
	}
}

func TestBootRegressionReportString(t *testing.T) {
	r := BootRegressionReport{Status: "pc=0x80000000", MemoryCounts: map[string]int{"elf": 1}, InitcallCounts: map[string]int{"virtio": 2}, TraceStats: TraceStats{Lines: 3, Steps: 2, Traps: 1}}
	s := BootRegressionReportString(r)
	for _, want := range []string{"status:", "memory objects", "initcall categories"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
}
