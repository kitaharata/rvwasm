package analyze

import (
	"strings"
	"testing"
)

func TestCompareSmokeSummaries(t *testing.T) {
	cur := []SmokeSummary{{Preset: "uart-blk", Ran: 20, Phase: "linux", TopCause: "none"}}
	base := []SmokeSummary{{Preset: "uart-blk", Ran: 10, Phase: "opensbi", TopCause: "trap"}}
	diff := CompareSmokeSummaries(cur, base)
	if len(diff) != 1 || diff[0].Kind != "changed" || diff[0].DeltaRan != 10 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
	if !strings.Contains(SmokeMatrixDiffString(diff), "uart-blk") {
		t.Fatalf("missing preset in string")
	}
}

func TestStopCauseChecklist(t *testing.T) {
	rows := []StopCauseCandidate{{Category: "virtqueue", Severity: "error", Summary: "bad queue"}}
	items := StopCauseChecklist(rows)
	if len(items) == 0 || !strings.Contains(items[0].Action, "Virtqueue") {
		t.Fatalf("unexpected checklist: %+v", items)
	}
}

func TestQueryBookmarkSet(t *testing.T) {
	hits := []QueryHit{{Source: "csr", Score: 10, Text: "satp"}, {Source: "mmio", Score: 9, Text: "QueueReady"}, {Source: "trace", Score: 8, Text: "pc=80200000"}, {Source: "console", Score: 7, Text: "ignored"}}
	set := BuildQueryBookmarkSet("satp", hits, 1)
	if len(set.CSRHits) != 1 || len(set.MMIOHits) != 1 || len(set.TraceHits) != 1 {
		t.Fatalf("unexpected bookmark set: %+v", set)
	}
}

func TestArtifactManifestString(t *testing.T) {
	a := NewArtifactEntry("firmware", []byte("abc"), 0x80000000, 0x80000000, false, "raw")
	m := ArtifactManifest{BootArgs: "console=ttyS0", HartCount: 1, NextAddr: 0x80200000, DTBAddr: 0x87e00000, Artifacts: []ArtifactEntry{a}}
	got := ArtifactManifestString(m)
	if !strings.Contains(got, "firmware") || !strings.Contains(got, "console=ttyS0") {
		t.Fatalf("bad manifest: %s", got)
	}
}
