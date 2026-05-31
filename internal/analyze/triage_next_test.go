package analyze

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestRankStopCausesPanicAndAnomaly(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x10000)
	console := "Linux version 6.x\nKernel panic - not syncing: VFS: unable to mount root fs\n"
	report := BuildBootRegressionReport("halted", console, "", b, nil, nil)
	rows := RankStopCauses("halted", console, "", report, []VirtqueueAnomaly{{Severity: "error", Device: "virtio-blk", Queue: 0, Detail: "ready queue has incomplete addresses desc=0x0 driver=0x0 device=0x0"}}, 4)
	if len(rows) == 0 || rows[0].Category != "kernel-panic" {
		t.Fatalf("expected panic first, got %#v", rows)
	}
	if rows[0].Rank != 1 || rows[0].Score == 0 {
		t.Fatalf("rank/score not set: %#v", rows[0])
	}
}

func TestPresetComparison(t *testing.T) {
	cur := []DiagnosticQueryPresetResult{{Preset: DiagnosticQueryPreset{Name: "panic", Query: "panic"}, Hits: []QueryHit{{Text: "panic"}}}}
	base := []DiagnosticQueryPresetResult{{Preset: DiagnosticQueryPreset{Name: "panic", Query: "panic"}}}
	rows := CompareDiagnosticQueryPresets(cur, base)
	if len(rows) != 1 || rows[0].Status != "new-hit" || rows[0].Delta != 1 {
		t.Fatalf("bad comparison: %#v", rows)
	}
}

func TestDumpMemoryIndexHitsString(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	if err := b.Load(0x80000010, []byte("Linux version test\x00")); err != nil {
		t.Fatal(err)
	}
	idx := BuildMemoryIndex(b, 8, 0x1000)
	s := DumpMemoryIndexHitsString(b, idx, "linux", 4, 32)
	if !strings.Contains(s, "Linux version") {
		t.Fatalf("dump missing ascii: %s", s)
	}
}

func TestRedactSensitiveText(t *testing.T) {
	s := RedactSensitiveText("mail a@b.example mac 02:72:76:77:00:01 ip 192.168.0.5", DefaultRedactionOptions())
	if strings.Contains(s, "a@b.example") || strings.Contains(s, "02:72") || strings.Contains(s, "192.168.0.5") {
		t.Fatalf("not redacted: %s", s)
	}
}

func TestTriageDashboardJSON(t *testing.T) {
	b := mem.NewBus(0x80000000, 0x1000)
	d := BuildTriageDashboard("running", "Linux version\n", "", "", b, nil, nil, "")
	if d.Phase == "" {
		t.Fatal("empty phase")
	}
	if _, err := json.Marshal(d); err != nil {
		t.Fatal(err)
	}
}
