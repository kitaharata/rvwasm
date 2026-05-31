package debugmap

import (
	"strings"
	"testing"
)

func TestAnnotateTraceText(t *testing.T) {
	tab := &Table{Symbols: []Symbol{{Addr: 0x80000000, Name: "start", Kind: "FUNC"}}, Lines: []Line{{Addr: 0x80000000, File: "start.S", Line: 7}}, Source: "test"}
	trace := "cycle=1 mode=3 pc=0000000080000000 inst=00000013 asm=\"addi x0,x0,0\" next=0000000080000004\n"
	got := tab.AnnotateTraceText(trace, 8)
	if !strings.Contains(got, "start") || !strings.Contains(got, "start.S:7") {
		t.Fatalf("unexpected annotation: %s", got)
	}
}

func TestLineSummary(t *testing.T) {
	tab := &Table{Lines: []Line{{Addr: 1, File: "a.c", Line: 1}, {Addr: 2, File: "a.c", Line: 2}, {Addr: 3, File: "b.c", Line: 1}}, Source: "test"}
	got := tab.LineSummary(4)
	if !strings.Contains(got, "a.c") || !strings.Contains(got, "2") {
		t.Fatalf("bad summary: %s", got)
	}
}
