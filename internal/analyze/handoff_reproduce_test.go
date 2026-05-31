package analyze

import (
	"strings"
	"testing"
)

func sampleBundleForRepro() DiagnosticBundle {
	m := ArtifactManifest{BootArgs: "console=ttyS0 root=/dev/vda", HartCount: 2, NextAddr: 0x80200000, Artifacts: []ArtifactEntry{{Role: "firmware", Bytes: 4, LoadAddr: 0x80000000, EndAddr: 0x80000003, Entry: 0x80000000, SHA256: "0123456789abcdef0123456789abcdef"}}}
	return DiagnosticBundle{Manifest: m, StopCauses: []StopCauseCandidate{{Rank: 1, Severity: "error", Score: 90, Category: "virtqueue anomaly", Summary: "queue descriptor outside DRAM", Evidence: []string{"pc=0x80200100 addr=0x10001050"}}}, Suggestions: []BreakpointSuggestion{{Kind: "pc-breakpoint", Address: 0x80200100, Command: "break pc=0x80200100", Reason: "test"}}}
}

func TestBuildReproductionPlan(t *testing.T) {
	b := sampleBundleForRepro()
	p := BuildReproductionPlan(b, ProvenanceReport{Tool: "rvwasm-test"}, "uart-blk")
	if p.Tool != "rvwasm-test" || p.HartCount != 2 || len(p.Steps) < 5 {
		t.Fatalf("unexpected plan: %+v", p)
	}
	s := ReproductionPlanString(p)
	if !strings.Contains(s, "Load pinned artifacts") || !strings.Contains(s, "console=ttyS0") {
		t.Fatalf("plan string missing details: %s", s)
	}
	if !strings.Contains(ReproductionPlanMarkdown(p), "Artifact pins") {
		t.Fatalf("markdown missing artifact pins")
	}
}

func TestLogSignatureSetAndDiff(t *testing.T) {
	m := sampleBundleForRepro().Manifest
	a := BuildLogSignatureSet("pc=0x80000000 addi\npc=0x80000004 ecall\n", "OpenSBI\nLinux\n", m)
	if a.TraceLines != 2 || a.FirstPC == "" || a.LastPC == "" || a.TraceSHA256 == "" {
		t.Fatalf("bad signature: %+v", a)
	}
	same := CompareLogSignatures(a, LogSignatureSetJSON(a))
	if len(same) != 1 || same[0].Kind != "unchanged" {
		t.Fatalf("expected unchanged, got %+v", same)
	}
	b := BuildLogSignatureSet("pc=0x80000000 addi\npc=0x80000008 trap\n", "OpenSBI\npanic\n", m)
	diff := CompareLogSignatures(b, LogSignatureSetJSON(a))
	if len(diff) == 0 || diff[0].Kind == "unchanged" {
		t.Fatalf("expected diff, got %+v", diff)
	}
	if !strings.Contains(LogSignatureDiffString(diff), "trace") {
		t.Fatalf("diff string missing trace: %s", LogSignatureDiffString(diff))
	}
}

func TestAppliedSuggestionReportAndHeadlessScript(t *testing.T) {
	b := sampleBundleForRepro()
	r := BuildAppliedSuggestionReport(b.Suggestions, 4)
	if len(r.Commands) != 1 || !strings.Contains(AppliedSuggestionReportString(r), "break pc") {
		t.Fatalf("bad report: %+v", r)
	}
	script := HeadlessSmokeScript(b.Manifest, []string{"uart-blk"}, 1234)
	if !strings.Contains(script, "STEPS=1234") || !strings.Contains(script, "sha256") {
		t.Fatalf("bad script: %s", script)
	}
}
