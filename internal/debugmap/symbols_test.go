package debugmap

import (
	"strings"
	"testing"
)

func TestParseSystemMapAndLookup(t *testing.T) {
	m := []byte("ffffffff80000000 T _start\nffffffff80001000 t setup_vm\nffffffff80002000 T start_kernel\n")
	tab, err := ParseSystemMap(m, "System.map")
	if err != nil {
		t.Fatal(err)
	}
	if tab.Count() != 3 {
		t.Fatalf("count=%d", tab.Count())
	}
	s, off, ok := tab.Lookup(0xffffffff80001020)
	if !ok || s.Name != "setup_vm" || off != 0x20 {
		t.Fatalf("lookup=%v off=%#x ok=%v", s, off, ok)
	}
	around := tab.Around(0xffffffff80001020, 1)
	if !strings.Contains(around, "setup_vm+0x20") || !strings.Contains(around, "start_kernel") {
		t.Fatalf("around output missing symbols:\n%s", around)
	}
}

func TestSymbolSearch(t *testing.T) {
	tab, err := ParseSystemMap([]byte("0000000080001000 T start_kernel\n0000000080002000 T rest_init\n"), "map")
	if err != nil {
		t.Fatal(err)
	}
	got := tab.Search("kernel", 8)
	if !strings.Contains(got, "start_kernel") || strings.Contains(got, "rest_init") {
		t.Fatalf("unexpected search output: %s", got)
	}
}

func TestAnalyzeLogResolvesAddresses(t *testing.T) {
	tab, err := ParseSystemMap([]byte("ffffffff80000000 T _start\nffffffff80001000 T panic_here\n"), "System.map")
	if err != nil {
		t.Fatal(err)
	}
	got := tab.AnalyzeLog("Oops at PC: ffffffff80001004 LR 0xffffffff80000000", 8)
	if !strings.Contains(got, "panic_here+0x4") || !strings.Contains(got, "_start") {
		t.Fatalf("AnalyzeLog output = %s", got)
	}
}

func TestLineLookupFormatting(t *testing.T) {
	tab := &Table{Source: "unit", Lines: []Line{{Addr: 0x1000, File: "start.c", Line: 10}, {Addr: 0x1010, File: "main.c", Line: 20}}}
	line, off, ok := tab.LookupLine(0x1014)
	if !ok || line.File != "main.c" || line.Line != 20 || off != 4 {
		t.Fatalf("line=%#v off=%#x ok=%v", line, off, ok)
	}
	if got := tab.AroundLine(0x1014, 1); !strings.Contains(got, "main.c:20+0x4") || !strings.Contains(got, "start.c:10") {
		t.Fatalf("AroundLine=%s", got)
	}
}
