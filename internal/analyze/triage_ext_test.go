package analyze

import (
	"strings"
	"testing"

	"github.com/kitaharata/rvwasm/internal/mem"
)

func TestStopCauseEvidenceString(t *testing.T) {
	rows := []StopCauseCandidate{{Rank: 1, Severity: "error", Score: 88, Category: "illegal-instruction", Summary: "illegal", Evidence: []string{"trap illegal"}, NextAction: "decode"}}
	s := StopCauseEvidenceString(rows)
	if !strings.Contains(s, "confidence") || !strings.Contains(s, "illegal") || !strings.Contains(s, "queries") {
		t.Fatalf("unexpected evidence string: %s", s)
	}
}

func TestRedactionOptionsFromJSON(t *testing.T) {
	opt := RedactionOptionsFromJSON(`{"replace_ips":false,"replace_long_hex":true}`, DefaultRedactionOptions())
	if opt.ReplaceIPs || !opt.ReplaceLongHex || !opt.ReplaceEmails || !opt.ReplaceMACs {
		t.Fatalf("bad options: %+v", opt)
	}
}

func TestDumpMemoryRangeString(t *testing.T) {
	b := mem.NewBus(0x80000000, 64)
	copy(b.DRAM[8:], []byte("hello rvwasm"))
	s := DumpMemoryRangeString(b, 0x80000008, 16)
	if !strings.Contains(s, "hello") || !strings.Contains(s, "80000008") {
		t.Fatalf("bad dump: %s", s)
	}
}

func TestTriageDashboardDiffString(t *testing.T) {
	before := `{"status":"old","phase":"opensbi","anomaly_counts":{"warn":1}}`
	after := TriageDashboard{Status: "new", Phase: "linux", AnomalyCounts: map[string]int{"warn": 2}}
	s := TriageDashboardDiffString(before, after)
	if !strings.Contains(s, "status") || !strings.Contains(s, "phase") || !strings.Contains(s, "anomaly_counts.warn") {
		t.Fatalf("missing diff: %s", s)
	}
}

func TestSmokeSummaryString(t *testing.T) {
	s := SmokeSummaryString([]SmokeSummary{{Preset: "uart-blk", Ran: 10, Requested: 20, Phase: "opensbi", TopCause: "none"}})
	if !strings.Contains(s, "uart-blk") || !strings.Contains(s, "opensbi") {
		t.Fatalf("bad smoke summary: %s", s)
	}
}
