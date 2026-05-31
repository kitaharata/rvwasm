package analyze

import "testing"

func TestDiagnosticQueryPresetResultsAndBookmarks(t *testing.T) {
	idx := MemoryIndex{Ranges: []MemoryIndexRange{{Start: 0x80200000, End: 0x80201000, Label: "linux-version-string=1", Objects: []MemoryObject{{Addr: 0x80200040, Type: "linux-version-string", Detail: "Linux version 6.x"}}}}}
	rows := DiagnosticQueryPresetResults("[ 0.0] Kernel panic - not syncing", "pc=0x80200000 trap cause=2", "satp write", nil, nil, idx, 8)
	if len(rows) == 0 {
		t.Fatal("no presets")
	}
	var panicHits, satpHits int
	for _, r := range rows {
		if r.Preset.Name == "panic/oops" {
			panicHits = len(r.Hits)
		}
		if r.Preset.Name == "CSR satp" {
			satpHits = len(r.Hits)
		}
	}
	if panicHits == 0 || satpHits == 0 {
		t.Fatalf("expected panic/satp hits, got panic=%d satp=%d", panicHits, satpHits)
	}
	if len(QueryBookmarks(rows, 1)) == 0 {
		t.Fatal("expected bookmarks")
	}
}

func TestVirtqueueAnomalyTriage(t *testing.T) {
	rows := []VirtqueueAnomaly{{Severity: "error", Device: "virtio-blk", Queue: 0, Detail: "descriptor 0 buffer outside DRAM addr=0x1 len=16"}, {Severity: "warn", Device: "virtio-blk", Queue: 0, Detail: "descriptor table is not 16-byte aligned: 0x1003"}}
	rep := VirtqueueAnomalyTriage(rows)
	if rep.Counts["critical"] != 1 || rep.Counts["warn"] != 1 {
		t.Fatalf("unexpected counts: %#v", rep.Counts)
	}
	if rep.Items[0].Severity != "critical" || rep.Items[0].Hint == "" {
		t.Fatalf("bad first triage item: %#v", rep.Items[0])
	}
	if got := VirtqueueAnomalyTriageString(rows); got == "" || got[0] == '<' {
		t.Fatalf("bad triage string: %q", got)
	}
}
